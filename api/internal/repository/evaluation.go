package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lib/pq"
	"github.com/lk16/flippy/api/internal/constants"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/redis/go-redis/v9"
)

// EvaluationRepository handles database operations for evaluations
type EvaluationRepository struct {
	services *services.Services
}

// NewEvaluationRepository creates a new EvaluationRepository
func NewEvaluationRepository(c *fiber.Ctx) *EvaluationRepository {
	return &EvaluationRepository{
		services: c.Locals("services").(*services.Services),
	}
}

func NewEvaluationRepositoryFromServices(services *services.Services) *EvaluationRepository {
	return &EvaluationRepository{
		services: services,
	}
}

// SubmitEvaluations submits a batch of evaluations
func (repo *EvaluationRepository) SubmitEvaluations(ctx context.Context, payload models.EvaluationsPayload) error {
	pgConn := repo.services.Postgres

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
		WITH current_level AS (
			SELECT level
			FROM edax
			WHERE position = $1
		)
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
		RETURNING
			(SELECT level FROM current_level) as old_level,
			level as new_level,
			disc_count;
	`, valuesClause)

	rows, err := pgConn.QueryxContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("error submitting evaluations: %w", err)
	}
	defer rows.Close()

	redisChanges := make(map[string]int)

	for rows.Next() {
		var oldLevel sql.NullInt64
		var newLevel, discCount int
		if err := rows.Scan(&oldLevel, &newLevel, &discCount); err != nil {
			return fmt.Errorf("error scanning evaluation: %w", err)
		}

		if oldLevel.Valid {
			redisChanges[fmt.Sprintf("%d:%d", discCount, oldLevel.Int64)]--
		}
		redisChanges[fmt.Sprintf("%d:%d", discCount, newLevel)]++
	}

	redisConn := repo.services.Redis

	// Update Redis in a single pipeline
	pipe := redisConn.Pipeline()
	for key, count := range redisChanges {
		pipe.HIncrBy(ctx, constants.BookStatsKey, key, int64(count))
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("error updating Redis stats: %w", err)
	}

	return nil
}

// LookupPositions looks up evaluations for given positions
func (repo *EvaluationRepository) LookupPositions(ctx context.Context, positions []models.NormalizedPosition) ([]models.Evaluation, error) {
	pgConn := repo.services.Postgres

	positionsBytes := make([][]byte, len(positions))
	for i, position := range positions {
		positionsBytes[i] = position.Bytes()
	}

	query := `
		SELECT position, level, depth, confidence, score, best_moves
		FROM edax
		WHERE position = ANY($1)
	`

	rows, err := pgConn.QueryxContext(ctx, query, pq.Array(positionsBytes))
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

func (repo *EvaluationRepository) RefreshBookStats(ctx context.Context) error {
	pgConn := repo.services.Postgres
	redisConn := repo.services.Redis

	query := `
		SELECT disc_count, level, count(*)
		FROM edax
		GROUP BY disc_count, level
	`

	type statRow struct {
		DiscCount int `db:"disc_count"`
		Level     int `db:"level"`
		Count     int `db:"count"`
	}

	var stats []statRow
	err := pgConn.SelectContext(ctx, &stats, query)
	if err != nil {
		return fmt.Errorf("error loading book stats: %w", err)
	}

	// Create a map to store the stats
	statsMap := make(map[string]interface{})
	for _, stat := range stats {
		key := fmt.Sprintf("%d:%d", stat.DiscCount, stat.Level)
		statsMap[key] = stat.Count
	}

	// Store in Redis hash
	err = redisConn.HSet(ctx, constants.BookStatsKey, statsMap).Err()
	if err != nil {
		return fmt.Errorf("error storing book stats in Redis: %w", err)
	}

	return nil
}

// GetBookStats returns statistics about positions in the database
func (repo *EvaluationRepository) GetBookStats(ctx context.Context) ([][]string, error) {
	redisConn := repo.services.Redis

	// Get all stats from Redis
	stats, err := redisConn.HGetAll(ctx, constants.BookStatsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("error getting book stats from Redis: %w", err)
	}

	discCounts := make(map[int]bool)
	levels := make(map[int]bool)
	lookup := make(map[[2]int]int)
	levelTotals := make(map[int]int)
	discTotals := make(map[int]int)

	for key, value := range stats {
		// Parse disc_count:level key
		var discCount, level int
		_, err := fmt.Sscanf(key, "%d:%d", &discCount, &level)
		if err != nil {
			return nil, fmt.Errorf("error parsing book stats key: %w", err)
		}

		// Parse count value
		count, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("error parsing book stats value: %w", err)
		}

		discCounts[discCount] = true
		levels[level] = true
		lookup[[2]int{discCount, level}] = count

		levelTotals[level] += count
		discTotals[discCount] += count
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

// RefillJobCache refreshes the cache of available jobs
func (repo *EvaluationRepository) RefillJobCache(ctx context.Context) error {

	pgConn := repo.services.Postgres
	redisConn := repo.services.Redis

	// Try to acquire the lock atomically with SetNX
	lockAcquired, err := redisConn.SetNX(ctx, constants.CacheRefreshLockKey, "1", constants.CacheRefreshLockTTL).Result()
	if err != nil {
		return fmt.Errorf("error acquiring cache refresh lock: %w", err)
	}

	if !lockAcquired {
		return nil
	}

	// Ensure lock is released
	defer func() {
		if err := redisConn.Del(ctx, constants.CacheRefreshLockKey).Err(); err != nil {
		}
	}()

	clientRepo := NewClientRepositoryFromServices(repo.services)
	takenPositionsBytes := clientRepo.GetTakenPositionsBytes(ctx)

	var positionBytes [][]byte

	// Try to find job at different disc counts, starting with the lowest
	for discCount := 4; discCount <= 64; discCount++ {
		learnLevel := models.LearnLevelFromDiscCount(discCount)
		query := `
			SELECT position
			FROM edax
			WHERE disc_count = $1
			AND level < $2
			AND position NOT IN ($3)
			ORDER BY RANDOM()
			LIMIT $4
		`

		limit := constants.JobRefreshSize - len(positionBytes)
		err := pgConn.SelectContext(ctx, &positionBytes, query, discCount, learnLevel, pq.Array(takenPositionsBytes), limit)

		if err == sql.ErrNoRows {
			continue
		}

		if err != nil {
			return fmt.Errorf("error getting positions from DB: %w", err)
		}

		if len(positionBytes) >= constants.JobRefreshSize {
			break
		}
	}

	positions := make([]models.NormalizedPosition, 0, len(positionBytes))
	for _, positionBytes := range positionBytes {
		position, err := models.NewNormalizedPositionFromBytes(positionBytes)
		if err != nil {
			return fmt.Errorf("error parsing position: %w", err)
		}
		positions = append(positions, position)
	}

	// Build list of positions to push to Redis
	positionStrings := make([]string, 0, len(positions))
	for _, pos := range positions {
		positionStrings = append(positionStrings, pos.String())
	}

	// Push new positions to Redis
	err = redisConn.RPush(ctx, constants.PositionsKey, positionStrings).Err()
	if err != nil {
		return fmt.Errorf("error pushing positions to Redis: %w", err)
	}

	return nil
}

// popJobFromRedis attempts to get a job from Redis and returns the number of remaining jobs
func (repo *EvaluationRepository) popJobFromRedis(ctx context.Context, clientID string) (models.Job, int, error) {
	redisConn := repo.services.Redis

	// Start a transaction
	tx := redisConn.TxPipeline()

	// Pop the job
	popCmd := tx.LPop(ctx, constants.PositionsKey)
	// Get new length
	lenCmd := tx.LLen(ctx, constants.PositionsKey)

	// Execute the transaction
	_, err := tx.Exec(ctx)
	if err != nil {
		return models.Job{}, 0, fmt.Errorf("error executing Redis transaction: %w", err)
	}

	// Get the popped position
	posStr, err := popCmd.Result()
	if err == redis.Nil {
		return models.Job{}, 0, ErrNoJobsAvailable
	}
	if err != nil {
		return models.Job{}, 0, fmt.Errorf("error getting position from Redis: %w", err)
	}

	// Get the new length
	remainingJobCount, err := lenCmd.Result()
	if err != nil {
		return models.Job{}, 0, fmt.Errorf("error getting remaining jobs count: %w", err)
	}

	position, err := models.NewNormalizedPositionFromString(posStr)
	if err != nil {
		return models.Job{}, 0, fmt.Errorf("error parsing position: %w", err)
	}

	job := models.Job{
		Position: position,
		Level:    position.TargetLearnLevel(),
	}

	clientRepo := NewClientRepositoryFromServices(repo.services)
	clientRepo.AssignJob(ctx, clientID, job)
	return job, int(remainingJobCount), nil
}

// GetJob retrieves the next available job from the database
func (repo *EvaluationRepository) GetJob(ctx context.Context, clientID string) (models.Job, error) {
	// Get job and remaining job count from Redis
	job, remainingJobCount, err := repo.popJobFromRedis(ctx, clientID)
	if err != nil && err != ErrNoJobsAvailable {
		return models.Job{}, fmt.Errorf("error getting job from Redis: %w", err)
	}

	// Refill cache if it's running low
	if remainingJobCount <= constants.RefillThreshold {
		go func() {
			refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = repo.RefillJobCache(refreshCtx)
			if err != nil {
				log.Printf("error refreshing job cache: %v", err)
			}
		}()
	}

	return job, err
}
