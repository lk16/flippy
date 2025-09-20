package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lk16/flippy/api/internal/othello"
	"github.com/redis/go-redis/v9"
)

const (
	positionsKey           = "positions_to_learn"
	positionsTTL           = 5 * time.Minute
	positionsMaxCount      = 500
	cacheRefreshLockKey    = "positions_to_learn_refresh_lock"
	cacheRefreshLockTTL    = 10 * time.Second
	cacheRefreshCtxTimeout = 10 * time.Second
	bookStatsKey           = "book_stats"

	clientsKey = "clients"
	clientsTTL = 300 * time.Second
)

// submitEvaluations submits a batch of evaluations.
func submitEvaluations(ctx context.Context, services *Services, payload EvaluationsPayload) error {
	if len(payload.Evaluations) == 0 {
		return nil
	}

	// Create a single VALUES clause with all the data
	valuesClause := ""
	params := make([]interface{}, 0, len(payload.Evaluations)*7)

	positionBytesList := make([][]byte, len(payload.Evaluations))
	for i, eval := range payload.Evaluations {
		positionBytesList[i] = eval.Position.Bytes()
	}

	for i, eval := range payload.Evaluations {
		if i > 0 {
			valuesClause += ", "
		}
		valuesClause += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i*7+2, i*7+3, i*7+4, i*7+5, i*7+6, i*7+7, i*7+8)

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

	// TODO remove edax.level from DB

	// Add positions array as first parameter
	params = append([]interface{}{pq.Array(positionBytesList)}, params...)

	// TODO Change table structure, we need to remove level as it is confusing.
	// TODO Simplify this code in the process.

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
		WHERE EXCLUDED.level > edax.level OR EXCLUDED.depth > edax.depth
		RETURNING
			(SELECT level FROM current_levels WHERE position = edax.position) as old_level,
			level as new_level,
			disc_count;
	`, valuesClause)

	rows, err := services.Postgres.QueryxContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("error submitting evaluations: %w", err)
	}
	if rows.Err() != nil {
		return fmt.Errorf("error submitting evaluations: %w", rows.Err())
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

	redisConn := services.Redis

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

// lookupPositions looks up evaluations for given positions.
func lookupPositions(
	ctx context.Context,
	services *Services,
	positions []othello.NormalizedPosition,
) ([]Evaluation, error) {
	pgConn := services.Postgres

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
	if rows.Err() != nil {
		return nil, fmt.Errorf("error submitting evaluations: %w", rows.Err())
	}
	defer rows.Close()

	evaluations := make([]Evaluation, 0)

	for rows.Next() {
		var evaluation Evaluation
		err = rows.StructScan(&evaluation)
		if err != nil {
			return nil, fmt.Errorf("error scanning evaluations: %w", err)
		}
		evaluations = append(evaluations, evaluation)
	}

	return evaluations, nil
}

func buildInitialBookStats(ctx context.Context, services *Services) error {
	pgConn := services.Postgres
	redisConn := services.Redis

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

// getBookStats returns statistics about positions in the database.
func getBookStats(ctx context.Context, services *Services) ([]BookStats, error) {
	redisConn := services.Redis

	// Get all stats from Redis
	stats, err := redisConn.HGetAll(ctx, bookStatsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("error getting book stats from Redis: %w", err)
	}

	if len(stats) == 0 {
		err = buildInitialBookStats(ctx, services)
		if err != nil {
			return nil, fmt.Errorf("error building initial book stats: %w", err)
		}

		// Try reading from Redis again after building stats
		stats, err = redisConn.HGetAll(ctx, bookStatsKey).Result()
		if err != nil {
			return nil, fmt.Errorf("error getting book stats from Redis after build: %w", err)
		}
	}

	bookStats := make([]BookStats, 0)

	for key, value := range stats {
		var bookStat BookStats

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
func tryRefreshJobCache(ctx context.Context, services *Services) error {
	// Try to acquire the lock for cache refresh
	redisConn := services.Redis
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

	learnableDiscCounts, err := getLearnableDiscCounts(ctx, services)
	if err != nil {
		return fmt.Errorf("error getting learnable disc counts: %w", err)
	}

	return refreshJobCache(ctx, services, learnableDiscCounts)
}

func getLearnableDiscCounts(ctx context.Context, services *Services) ([]int, error) {
	bookStats, err := getBookStats(ctx, services)
	if err != nil {
		return nil, fmt.Errorf("error getting book stats: %w", err)
	}

	// Find disc counts that have positions needing work, use map[int]bool as set.
	learnableDiscCountsMap := make(map[int]bool)
	for _, bookStat := range bookStats {
		if bookStat.Level < GetLearnLevel(bookStat.DiscCount) {
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

func refreshJobCache(ctx context.Context, services *Services, learnableDiscCounts []int) error {
	redisConn := services.Redis

	takenPositionsBytes := getTakenPositionsBytes(ctx, services)

	pgConn := services.Postgres

	// Try to find job at different disc counts, starting with the lowest
	var positionBytes [][]byte
	for _, dc := range learnableDiscCounts {
		learnLevel := GetLearnLevel(dc)
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
		nPos, err := othello.NewNormalizedPositionFromBytes(positionBytes)
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
func tryPopJob(ctx context.Context, services *Services, clientID string) (Job, error) {
	redisConn := services.Redis

	posStr, err := redisConn.LPop(ctx, positionsKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return Job{}, ErrNoJobsAvailable
		}

		return Job{}, fmt.Errorf("error getting position from Redis: %w", err)
	}

	position, err := othello.NewNormalizedPositionFromString(posStr)
	if err != nil {
		return Job{}, fmt.Errorf("error parsing position: %w", err)
	}

	job := Job{
		Position: position,
		Level:    GetLearnLevel(position.CountDiscs()),
	}

	err = assignJob(ctx, services, clientID, job)
	if err != nil {
		return Job{}, fmt.Errorf("error assigning job: %w", err)
	}

	return job, nil
}

// getJob retrieves the next available job from the database.
func getJob(ctx context.Context, services *Services, clientID string) (Job, error) {
	// First try to get a position from Redis
	job, err := tryPopJob(ctx, services, clientID)

	if err != nil {
		if errors.Is(err, ErrNoJobsAvailable) {
			// Refresh the job cache in the background
			go func() {
				refreshCtx, cancel := context.WithTimeout(ctx, cacheRefreshCtxTimeout)
				defer cancel()

				err = tryRefreshJobCache(refreshCtx, services)
				if err != nil {
					slog.Error("error refreshing job cache", "error", err)
				}
			}()

			// Tell client to try again later
			return Job{}, ErrNoJobsAvailable
		}

		// Some other error occurred
		return Job{}, err
	}

	// No error, return job
	return job, nil
}

// GetLearnLevel returns the target level for a given disc count.
func GetLearnLevel(discCount int) int {
	// TODO move this out

	if discCount <= 9 {
		return 44
	}

	if discCount <= 13 {
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

// registerClient registers a new client and returns its ID.
func registerClient(ctx context.Context, services *Services, req RegisterRequest) (*RegisterResponse, error) {
	clientID := uuid.New().String()

	clientStats := ClientStats{
		ID:                clientID,
		Hostname:          req.Hostname,
		GitCommit:         req.GitCommit,
		PositionsComputed: 0,
		LastActive:        time.Now(),
		Position:          othello.NewNormalizedPositionEmpty(),
	}

	// Convert to JSON
	jsonData, err := json.Marshal(clientStats)
	if err != nil {
		return nil, fmt.Errorf("error marshaling client stats: %w", err)
	}

	redisConn := services.Redis

	// Store in Redis hash and set TTL
	err = redisConn.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return nil, fmt.Errorf("error storing client: %w", err)
	}

	// Set TTL on the hash
	err = redisConn.Expire(ctx, clientsKey, clientsTTL).Err()
	if err != nil {
		return nil, fmt.Errorf("error setting TTL: %w", err)
	}

	return &RegisterResponse{ClientID: clientID}, nil
}

// updateHeartbeat updates the last_heartbeat timestamp for a client.
func updateHeartbeat(ctx context.Context, services *Services, clientID string) error {
	redisConn := services.Redis

	// Get current client stats
	jsonData, err := redisConn.HGet(ctx, clientsKey, clientID).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("client %s not found", clientID)
		}
		return fmt.Errorf("error getting client: %w", err)
	}

	var clientStats ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	// Update last active time
	clientStats.LastActive = time.Now()

	// Convert back to JSON
	jsonData, err = json.Marshal(clientStats)
	if err != nil {
		return fmt.Errorf("error marshaling client stats: %w", err)
	}

	// Update in Redis and reset TTL
	err = redisConn.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = redisConn.Expire(ctx, clientsKey, clientsTTL).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// getClientStatsList retrieves statistics for all clients.
func getClientStatsList(ctx context.Context, services *Services) (StatsResponse, error) {
	redisConn := services.Redis

	// Get all clients from Redis hash
	clients, err := redisConn.HGetAll(ctx, clientsKey).Result()
	if err != nil {
		return StatsResponse{}, fmt.Errorf("error getting clients: %w", err)
	}

	stats := make([]ClientStats, 0, len(clients))
	for _, jsonData := range clients {
		var clientStats ClientStats

		if err = json.Unmarshal([]byte(jsonData), &clientStats); err != nil {
			return StatsResponse{}, fmt.Errorf("error unmarshaling client stats: %w", err)
		}
		stats = append(stats, clientStats)
	}

	// Sort such that the most recently active clients are first
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].LastActive.After(stats[j].LastActive)
	})

	return StatsResponse{
		ActiveClients: len(stats),
		ClientStats:   stats,
	}, nil
}

var ErrClientNotFound = errors.New("client not found")

// getClientStats retrieves statistics for a specific client.
func getClientStats(ctx context.Context, services *Services, clientID string) (*ClientStats, error) {
	redisConn := services.Redis

	jsonData, err := redisConn.HGet(ctx, clientsKey, clientID).Bytes()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrClientNotFound
		}

		return nil, fmt.Errorf("error getting client: %w", err)
	}

	var clientStats ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	return &clientStats, nil
}

// assignJob assigns a job to a client.
func assignJob(ctx context.Context, services *Services, clientID string, job Job) error {
	redisConn := services.Redis

	// Get current client stats
	jsonData, err := redisConn.HGet(ctx, clientsKey, clientID).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("client %s not found", clientID)
		}
		return fmt.Errorf("error getting client: %w", err)
	}

	var clientStats ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	// Update position
	clientStats.Position = job.Position

	// Convert back to JSON
	jsonData, err = json.Marshal(clientStats)
	if err != nil {
		return fmt.Errorf("error marshaling client stats: %w", err)
	}

	// Update in Redis and reset TTL
	err = redisConn.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = redisConn.Expire(ctx, clientsKey, clientsTTL).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// CompleteJob marks a job as completed and updates client stats.
func CompleteJob(ctx context.Context, services *Services, clientID string) error {
	redisConn := services.Redis
	// Get current client stats
	jsonData, err := redisConn.HGet(ctx, clientsKey, clientID).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("client %s not found", clientID)
		}
		return fmt.Errorf("error getting client: %w", err)
	}

	var clientStats ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	// Update positions computed
	clientStats.PositionsComputed++

	// Convert back to JSON
	jsonData, err = json.Marshal(clientStats)
	if err != nil {
		return fmt.Errorf("error marshaling client stats: %w", err)
	}

	// Update in Redis and reset TTL
	err = redisConn.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = redisConn.Expire(ctx, clientsKey, clientsTTL).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// getTakenPositionsBytes returns all the positions that have been taken by clients.
func getTakenPositionsBytes(ctx context.Context, services *Services) [][]byte {
	redisConn := services.Redis

	// Get all clients from Redis hash
	clients, err := redisConn.HGetAll(ctx, clientsKey).Result()
	if err != nil {
		return nil
	}

	takenPositionsBytes := make([][]byte, 0, len(clients))
	for _, jsonData := range clients {
		var clientStats ClientStats
		if err = json.Unmarshal([]byte(jsonData), &clientStats); err != nil {
			continue
		}

		takenPositionsBytes = append(takenPositionsBytes, clientStats.Position.Bytes())
	}

	return takenPositionsBytes
}
