package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/lk16/flippy/api/internal/db"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/redis/go-redis/v9"
)

const (
	positionsKey = "positions_to_learn"
	positionsTTL = 5 * time.Minute
)

// EvaluationRepository handles database operations for evaluations
type EvaluationRepository struct {
	db         *sqlx.DB
	clientRepo *ClientRepository
	redis      *redis.Client
}

// NewEvaluationRepository creates a new EvaluationRepository
func NewEvaluationRepository(clientRepo *ClientRepository, redis *redis.Client) *EvaluationRepository {
	return &EvaluationRepository{
		db:         db.GetDB(),
		clientRepo: clientRepo,
		redis:      redis,
	}
}

// SubmitEvaluations submits a batch of evaluations
func (r *EvaluationRepository) SubmitEvaluations(ctx context.Context, payload models.EvaluationsPayload) error {
	if len(payload.Evaluations) == 0 {
		return nil
	}

	// Create a single VALUES clause with all the data
	valuesClause := ""
	params := make([]interface{}, 0, len(payload.Evaluations)*7)

	for i, eval := range payload.Evaluations {
		if i > 0 {
			valuesClause += ", "
		}
		valuesClause += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i*7+1, i*7+2, i*7+3, i*7+4, i*7+5, i*7+6, i*7+7)

		params = append(params,
			eval.Position.Bytes(),
			eval.Position.CountDiscs(),
			eval.Level,
			eval.Depth,
			eval.Confidence,
			eval.Score,
			pq.Array(eval.BestMoves),
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO edax (position, disc_count, level, depth, confidence, score, best_moves)
		VALUES %s
		ON CONFLICT (position)
		DO UPDATE SET
			level = EXCLUDED.level,
			depth = EXCLUDED.depth,
			confidence = EXCLUDED.confidence,
			score = EXCLUDED.score,
			best_moves = EXCLUDED.best_moves
		WHERE EXCLUDED.level > edax.level
	`, valuesClause)

	_, err := r.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("error submitting evaluations: %w", err)
	}

	return nil
}

// LookupPositions looks up evaluations for given positions
func (r *EvaluationRepository) LookupPositions(ctx context.Context, positions []models.NormalizedPosition) ([]models.Evaluation, error) {
	positionsBytes := make([][]byte, len(positions))
	for i, position := range positions {
		positionsBytes[i] = position.Bytes()
	}

	query := `
		SELECT position, level, depth, confidence, score, best_moves
		FROM edax
		WHERE position = ANY($1)
	`

	rows, err := r.db.QueryxContext(ctx, query, pq.Array(positionsBytes))
	if err != nil {
		return nil, fmt.Errorf("error looking up positions: %w", err)
	}
	defer rows.Close()

	evaluations := make([]models.Evaluation, 0)
	for rows.Next() {
		var eval models.Evaluation
		if err := eval.ScanRow(rows); err != nil {
			return nil, fmt.Errorf("error scanning evaluation: %w", err)
		}
		evaluations = append(evaluations, eval)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return evaluations, nil
}

// RefreshStatsView refreshes the materialized view for statistics
func (r *EvaluationRepository) RefreshStatsView(ctx context.Context) error {
	query := `REFRESH MATERIALIZED VIEW edax_stats_view`

	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("error refreshing stats view: %w", err)
	}

	return nil
}

// GetBookStats returns statistics about positions in the database
func (r *EvaluationRepository) GetBookStats(ctx context.Context) ([][]string, error) {
	query := `
		SELECT disc_count, level, count
		FROM edax_stats_view
		ORDER BY disc_count, level
	`

	type statRow struct {
		DiscCount int `db:"disc_count"`
		Level     int `db:"level"`
		Count     int `db:"count"`
	}

	var stats []statRow
	err := r.db.SelectContext(ctx, &stats, query)
	if err != nil {
		return nil, fmt.Errorf("error getting book stats: %w", err)
	}

	// Group stats by disc count and level
	discCounts := make(map[int]bool)
	levels := make(map[int]bool)
	lookup := make(map[[2]int]int)
	levelTotals := make(map[int]int)
	discTotals := make(map[int]int)

	for _, row := range stats {
		discCounts[row.DiscCount] = true
		levels[row.Level] = true
		lookup[[2]int{row.DiscCount, row.Level}] = row.Count

		levelTotals[row.Level] += row.Count
		discTotals[row.DiscCount] += row.Count
	}

	// Convert sets to sorted slices
	discCountsSorted := make([]int, 0, len(discCounts))
	for dc := range discCounts {
		discCountsSorted = append(discCountsSorted, dc)
	}
	sort.Ints(discCountsSorted)

	levelsSorted := make([]int, 0, len(levels))
	for l := range levels {
		levelsSorted = append(levelsSorted, l)
	}
	sort.Ints(levelsSorted)

	// Build the table
	table := make([][]string, 0, len(discCountsSorted)+2)

	// Header row
	header := []string{""}
	for _, level := range levelsSorted {
		header = append(header, fmt.Sprintf("level %d", level))
	}
	header = append(header, "Total")
	table = append(table, header)

	// Data rows
	for _, discs := range discCountsSorted {
		row := []string{fmt.Sprintf("%d discs", discs)}
		for _, level := range levelsSorted {
			count := lookup[[2]int{discs, level}]
			row = append(row, fmt.Sprintf("%d", count))
		}
		row = append(row, fmt.Sprintf("%d", discTotals[discs]))
		table = append(table, row)
	}

	// Total row
	totalRow := []string{"Total"}
	for _, level := range levelsSorted {
		totalRow = append(totalRow, fmt.Sprintf("%d", levelTotals[level]))
	}
	totalRow = append(totalRow, fmt.Sprintf("%d", sum(levelTotals)))
	table = append(table, totalRow)

	return table, nil
}

func sum(m map[int]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// ErrNoJobsAvailable is returned when there are no more jobs to process
var ErrNoJobsAvailable = errors.New("no jobs available")

// refreshCachedAvailableJobs refreshes the cache of available jobs
func (r *EvaluationRepository) refreshCachedAvailableJobs(ctx context.Context) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Find disc counts that have positions needing work
	query := `
		SELECT disc_count, level
		FROM edax_stats_view
		WHERE count > 0
		ORDER BY disc_count, level
	`

	type statRow struct {
		DiscCount int `db:"disc_count"`
		Level     int `db:"level"`
	}

	var rows []statRow
	err = tx.SelectContext(ctx, &rows, query)
	if err != nil {
		return fmt.Errorf("error getting stats: %w", err)
	}

	// Find learnable disc counts
	learnableDiscCounts := make([]int, 0)
	for _, row := range rows {
		if row.Level < getLearnLevel(row.DiscCount) {
			learnableDiscCounts = append(learnableDiscCounts, row.DiscCount)
		}
	}

	takenPositionsBytes := r.clientRepo.GetTakenPositionsBytes(ctx)

	// Try to find job at different disc counts, starting with the lowest
	var positionBytes [][]byte
	for _, dc := range learnableDiscCounts {
		learnLevel := getLearnLevel(dc)
		query = `
			SELECT position
			FROM edax
			WHERE disc_count = $1
			AND level < $2
			AND position NOT IN ($3)
			ORDER BY RANDOM()
			LIMIT 500
		`

		err = tx.SelectContext(ctx, &positionBytes, query, dc, learnLevel, pq.Array(takenPositionsBytes))

		if err == sql.ErrNoRows {
			continue
		}

		if err != nil {
			return fmt.Errorf("error getting position: %w", err)
		}

		break
	}

	positions := make([]models.NormalizedPosition, 0, len(positionBytes))
	for _, positionBytes := range positionBytes {
		position, err := models.NewNormalizedPositionFromBytes(positionBytes)
		if err != nil {
			return fmt.Errorf("error parsing position: %w", err)
		}
		positions = append(positions, position)
	}

	// Convert positions to strings and push to Redis
	positionStrings := make([]string, 0, len(positions))
	for _, pos := range positions {
		positionStrings = append(positionStrings, pos.String())
	}

	if len(positionStrings) > 0 {
		// Delete existing list and set new values atomically
		err = r.redis.Del(ctx, positionsKey).Err()
		if err != nil {
			return fmt.Errorf("error deleting positions from Redis: %w", err)
		}

		// Push new positions to Redis
		err = r.redis.RPush(ctx, positionsKey, positionStrings).Err()
		if err != nil {
			return fmt.Errorf("error pushing positions to Redis: %w", err)
		}

		// Set TTL
		err = r.redis.Expire(ctx, positionsKey, positionsTTL).Err()
		if err != nil {
			return fmt.Errorf("error setting Redis TTL: %w", err)
		}
	}

	return nil
}

// createJobFromPosition creates a job from a position string
func (r *EvaluationRepository) createJobFromPosition(ctx context.Context, clientID string, posStr string) (models.Job, error) {
	position, err := models.NewNormalizedPositionFromString(posStr)
	if err != nil {
		return models.Job{}, fmt.Errorf("error parsing position: %w", err)
	}

	job := models.Job{
		Position: position,
		Level:    getLearnLevel(position.CountDiscs()),
	}

	r.clientRepo.AssignJob(ctx, clientID, job)
	return job, nil
}

// GetJob retrieves the next available job from the database
func (r *EvaluationRepository) GetJob(ctx context.Context, clientID string) (models.Job, error) {
	// First try to get a position from Redis
	posStr, err := r.redis.LPop(ctx, positionsKey).Result()
	if err == nil {
		return r.createJobFromPosition(ctx, clientID, posStr)
	}

	if err != redis.Nil {
		return models.Job{}, fmt.Errorf("error getting position from Redis: %w", err)
	}

	// If Redis is empty, try to refresh the cache
	err = r.refreshCachedAvailableJobs(ctx)
	if err != nil {
		return models.Job{}, fmt.Errorf("error refreshing job cache: %w", err)
	}

	// Try Redis again after refresh
	posStr, err = r.redis.LPop(ctx, positionsKey).Result()
	if err == nil {
		return r.createJobFromPosition(ctx, clientID, posStr)
	}

	if err == redis.Nil {
		return models.Job{}, ErrNoJobsAvailable
	}

	return models.Job{}, fmt.Errorf("error getting position from Redis: %w", err)
}

// getLearnLevel returns the target level for a given disc count
func getLearnLevel(discCount int) int {
	if discCount <= 12 {
		return 40
	}

	if discCount <= 16 {
		return 36
	}

	if discCount <= 20 {
		return 34
	}

	return 32
}
