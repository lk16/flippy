package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lib/pq"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/redis/go-redis/v9"
)

const (
	positionsKey           = "positions_to_learn"
	positionsTTL           = 5 * time.Minute
	positionsMaxCount      = 500
	cacheRefreshLockKey    = "positions_to_learn_refresh_lock"
	cacheRefreshLockTTL    = 10 * time.Second
	cacheRefreshCtxTimeout = 10 * time.Second
	popRetryInterval       = 200 * time.Millisecond
	bookStatsKey           = "book_stats"
)

// EvaluationRepository handles database operations for evaluations.
type EvaluationRepository struct {
	services *services.Services
}

// NewEvaluationRepository creates a new EvaluationRepository.
func NewEvaluationRepository(c *fiber.Ctx) *EvaluationRepository {
	services := c.Locals("services").(*services.Services) //nolint: errcheck

	return &EvaluationRepository{
		services: services,
	}
}

func NewEvaluationRepositoryFromServices(services *services.Services) *EvaluationRepository {
	return &EvaluationRepository{
		services: services,
	}
}

// SubmitEvaluations submits a batch of evaluations.
func (repo *EvaluationRepository) SubmitEvaluations(ctx context.Context, payload models.EvaluationsPayload) error {
	pgConn := repo.services.Postgres

	if len(payload.Evaluations) == 0 {
		return nil
	}

	// Create a single VALUES clause with all the data
	valuesClause := ""
	params := make([]interface{}, 0, len(payload.Evaluations)*7) //nolint:mnd

	positionBytesList := make([][]byte, len(payload.Evaluations))
	for i, eval := range payload.Evaluations {
		positionBytesList[i] = eval.Position.Bytes()
	}

	for i, eval := range payload.Evaluations {
		if i > 0 {
			valuesClause += ", "
		}
		valuesClause += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i*7+2, i*7+3, i*7+4, i*7+5, i*7+6, i*7+7, i*7+8) //nolint:mnd

		params = append(params,
			positionBytesList[i],
			eval.Position.CountDiscs(),
			eval.Level,
			eval.Depth,
			eval.Confidence,
			eval.Score,
			pq.Array(eval.BestMoves),
		)
	}

	// Add positions array as first parameter
	params = append([]interface{}{pq.Array(positionBytesList)}, params...)

	query := fmt.Sprintf(`
		WITH current_levels AS (
			SELECT position, level
			FROM edax
			WHERE position = ANY($1)
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
			(SELECT level FROM current_levels WHERE position = edax.position) as old_level,
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
		if err = rows.Scan(&oldLevel, &newLevel, &discCount); err != nil {
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
		pipe.HIncrBy(ctx, bookStatsKey, key, int64(count))
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("error updating Redis stats: %w", err)
	}

	return nil
}

// LookupPositions looks up evaluations for given positions.
func (repo *EvaluationRepository) LookupPositions(
	ctx context.Context,
	positions []models.NormalizedPosition,
) ([]models.Evaluation, error) {
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
		var evaluation models.Evaluation
		err = rows.StructScan(&evaluation)
		if err != nil {
			return nil, fmt.Errorf("error scanning evaluations: %w", err)
		}
		evaluations = append(evaluations, evaluation)
	}

	return evaluations, nil
}

func (repo *EvaluationRepository) buildInitialBookStats(ctx context.Context) error {
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
	err = redisConn.HSet(ctx, bookStatsKey, statsMap).Err()
	if err != nil {
		return fmt.Errorf("error storing book stats in Redis: %w", err)
	}

	return nil
}

// GetBookStats returns statistics about positions in the database.
func (repo *EvaluationRepository) GetBookStats(ctx context.Context) ([]models.BookStats, error) {
	redisConn := repo.services.Redis

	// Get all stats from Redis
	stats, err := redisConn.HGetAll(ctx, bookStatsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("error getting book stats from Redis: %w", err)
	}

	if len(stats) == 0 {
		err = repo.buildInitialBookStats(ctx)
		if err != nil {
			return nil, fmt.Errorf("error building initial book stats: %w", err)
		}

		// Try reading from Redis again after building stats
		stats, err = redisConn.HGetAll(ctx, bookStatsKey).Result()
		if err != nil {
			return nil, fmt.Errorf("error getting book stats from Redis after build: %w", err)
		}
	}

	bookStats := make([]models.BookStats, 0)

	for key, value := range stats {
		var bookStat models.BookStats

		// Parse disc_count:level key
		if _, err = fmt.Sscanf(key, "%d:%d", &bookStat.DiscCount, &bookStat.Level); err != nil {
			return nil, fmt.Errorf("error parsing book stats key: %w", err)
		}

		// Parse count value
		bookStat.Count, err = strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("error parsing book stats value: %w", err)
		}

		bookStats = append(bookStats, bookStat)
	}

	return bookStats, nil
}

// ErrNoJobsAvailable is returned when there are no more jobs to process.
var ErrNoJobsAvailable = errors.New("no jobs available")

// tryRefreshJobCache refreshes the cache of available jobs.
func (repo *EvaluationRepository) tryRefreshJobCache(ctx context.Context) error {
	// Try to acquire the lock for cache refresh
	redisConn := repo.services.Redis
	lockAcquired, err := redisConn.SetNX(ctx, cacheRefreshLockKey, "1", cacheRefreshLockTTL).Result()

	if err != nil {
		return fmt.Errorf("error acquiring cache refresh lock: %w", err)
	}

	if !lockAcquired {
		// Someone else is refreshing the cache. Tell client to try again later.
		return nil
	}

	// We got the lock, we're responsible for refreshing the cache

	// Ensure lock is released
	defer redisConn.Del(ctx, cacheRefreshLockKey)

	learnableDiscCounts, err := repo.getLearnableDiscCounts(ctx)
	if err != nil {
		return fmt.Errorf("error getting learnable disc counts: %w", err)
	}

	return repo.refreshJobCache(ctx, learnableDiscCounts)
}

func (repo *EvaluationRepository) getLearnableDiscCounts(ctx context.Context) ([]int, error) {
	bookStats, err := repo.GetBookStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting book stats: %w", err)
	}

	// Find disc counts that have positions needing work, use map[int]bool as set.
	learnableDiscCountsMap := make(map[int]bool)
	for _, bookStat := range bookStats {
		if bookStat.Level < getLearnLevel(bookStat.DiscCount) {
			learnableDiscCountsMap[bookStat.DiscCount] = true
		}
	}

	// Convert map to slice and sort
	learnableDiscCounts := make([]int, 0, len(learnableDiscCountsMap))
	for discCount := range learnableDiscCountsMap {
		learnableDiscCounts = append(learnableDiscCounts, discCount)
	}
	sort.Ints(learnableDiscCounts)

	return learnableDiscCounts, nil
}

func (repo *EvaluationRepository) refreshJobCache(ctx context.Context, learnableDiscCounts []int) error {
	redisConn := repo.services.Redis

	clientRepo := NewClientRepositoryFromServices(repo.services)
	takenPositionsBytes := clientRepo.GetTakenPositionsBytes(ctx)

	pgConn := repo.services.Postgres

	// Try to find job at different disc counts, starting with the lowest
	var positionBytes [][]byte
	for _, dc := range learnableDiscCounts {
		learnLevel := getLearnLevel(dc)
		query := `
			SELECT position
			FROM edax
			WHERE disc_count = $1
			AND level < $2
			AND position NOT IN ($3)
			ORDER BY RANDOM()
			LIMIT $4
		`

		remaining := positionsMaxCount - len(positionBytes)
		if remaining <= 0 {
			break
		}

		var newPositions [][]byte
		err := pgConn.SelectContext(ctx, &newPositions, query, dc, learnLevel, pq.Array(takenPositionsBytes), remaining)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}

			return fmt.Errorf("error getting position: %w", err)
		}

		positionBytes = append(positionBytes, newPositions...)
	}

	positionStrings := make([]string, 0, len(positionBytes))
	for _, positionBytes := range positionBytes {
		nPos, err := models.NewNormalizedPositionFromBytes(positionBytes)
		if err != nil {
			return fmt.Errorf("error parsing position: %w", err)
		}

		// Convert positions to strings
		positionStrings = append(positionStrings, nPos.String())
	}

	if len(positionStrings) > 0 {
		// Delete existing list and set new values atomically
		err := redisConn.Del(ctx, positionsKey).Err()
		if err != nil {
			return fmt.Errorf("error deleting positions from Redis: %w", err)
		}

		// Push new positions to Redis
		err = redisConn.RPush(ctx, positionsKey, positionStrings).Err()
		if err != nil {
			return fmt.Errorf("error pushing positions to Redis: %w", err)
		}

		// Set TTL
		err = redisConn.Expire(ctx, positionsKey, positionsTTL).Err()
		if err != nil {
			return fmt.Errorf("error setting Redis TTL: %w", err)
		}
	}

	return nil
}

// tryPopJob attempts to get a job from Redis.
func (repo *EvaluationRepository) tryPopJob(ctx context.Context, clientID string) (models.Job, error) {
	redisConn := repo.services.Redis

	posStr, err := redisConn.LPop(ctx, positionsKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return models.Job{}, ErrNoJobsAvailable
		}

		return models.Job{}, fmt.Errorf("error getting position from Redis: %w", err)
	}

	position, err := models.NewNormalizedPositionFromString(posStr)
	if err != nil {
		return models.Job{}, fmt.Errorf("error parsing position: %w", err)
	}

	job := models.Job{
		Position: position,
		Level:    getLearnLevel(position.CountDiscs()),
	}

	clientRepo := NewClientRepositoryFromServices(repo.services)

	err = clientRepo.AssignJob(ctx, clientID, job)
	if err != nil {
		return models.Job{}, fmt.Errorf("error assigning job: %w", err)
	}

	return job, nil
}

// GetJob retrieves the next available job from the database.
func (repo *EvaluationRepository) GetJob(ctx context.Context, clientID string) (models.Job, error) {
	// First try to get a position from Redis
	job, err := repo.tryPopJob(ctx, clientID)

	if err != nil {
		if errors.Is(err, ErrNoJobsAvailable) {
			// Refresh the job cache in the background
			go func() {
				refreshCtx, cancel := context.WithTimeout(ctx, cacheRefreshCtxTimeout)
				defer cancel()

				err = repo.tryRefreshJobCache(refreshCtx)
				if err != nil {
					slog.Error("error refreshing job cache", "error", err)
				}
			}()

			// Tell client to try again later
			return models.Job{}, ErrNoJobsAvailable
		}

		// Some other error occurred
		return models.Job{}, err
	}

	// No error, return job
	return job, nil
}

// getLearnLevel returns the target level for a given disc count.
func getLearnLevel(discCount int) int {
	if discCount <= 9 { //nolint:mnd
		return 44 //nolint:mnd
	}

	if discCount <= 13 { //nolint:mnd
		return 40 //nolint:mnd
	}

	if discCount <= 16 { //nolint:mnd
		return 36 //nolint:mnd
	}

	if discCount <= 20 { //nolint:mnd
		return 34 //nolint:mnd
	}

	return 32 //nolint:mnd
}
