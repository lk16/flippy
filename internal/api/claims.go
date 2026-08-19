package api

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// claimTTL is how long a job claim or worker hash survives without a refresh.
const claimTTL = 5 * time.Minute

// jobFloorKey caches the lowest disc count that might still have unclaimed work, so claimJob can skip
// re-scanning disc counts already known to be fully learned.
const jobFloorKey = "job_floor_disc_count"

// jobFloorTTL bounds how long the cached job floor is trusted before a claim re-derives it from
// scratch, so boards an import (see internal/loader) adds below the floor are eventually rediscovered.
const jobFloorTTL = 10 * time.Minute

// statsKey caches the JSON response for GET /api/stats, which aggregates all rows in the boards table
// and becomes slow as the table grows to millions of rows.
const statsKey = "stats"

// statsTTL bounds how long the stats cache is trusted. Explicit invalidation (on each SaveEvaluation)
// keeps it fresh in the common case; the TTL is a safety net so loader-added boards eventually appear
// even without an explicit invalidation.
const statsTTL = 60 * time.Second

// Redis field names within a worker's hash (see workerKey).
const (
	workerFieldHostname          = "hostname"
	workerFieldGitCommit         = "git_commit"
	workerFieldPositionsComputed = "positions_computed"
	workerFieldLastActive        = "last_active"
)

// claimKey is the redis key holding the worker ID that currently holds board's job, if any.
func claimKey(board string) string {
	return "claim:" + board
}

// workerKey is the redis key of the hash describing a worker's hostname, git commit, stats, and claimed board.
func workerKey(workerID string) string {
	return "worker:" + workerID
}

// tryClaim atomically claims board for workerID, reporting false with no error if already claimed.
func (s *Server) tryClaim(ctx context.Context, board, workerID string) (bool, error) {
	ok, err := s.redis.SetNX(ctx, claimKey(board), workerID, claimTTL).Result()
	if err != nil {
		return false, fmt.Errorf("failed to claim job: %w", err)
	}
	if !ok {
		return false, nil
	}

	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, workerKey(workerID),
		workerFieldLastActive, time.Now().Format(time.RFC3339Nano),
	)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("failed to record worker claim: %w", err)
	}

	return true, nil
}

// releaseClaim releases workerID's claim on board; best-effort since claims also expire via claimTTL.
// The claim key is only deleted if it still belongs to workerID, so a late release can't revoke a claim
// another worker took over after this one's TTL expired. GET-then-DEL is not atomic: another worker
// could re-claim between the two, and the DEL would revoke its claim. Worst case is one duplicated
// evaluation, which is acceptable.
func (s *Server) releaseClaim(ctx context.Context, board, workerID string) error {
	owner, err := s.redis.Get(ctx, claimKey(board)).Result()
	if err == redis.Nil || (err == nil && owner != workerID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to release claim: %w", err)
	}

	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, claimKey(board))
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to release claim: %w", err)
	}

	return nil
}

// getJobFloor returns the cached lowest disc count worth querying for a job, or fallback if the cache
// is unset, expired, or holds an unreadable value.
func (s *Server) getJobFloor(ctx context.Context, fallback int) (int, error) {
	value, err := s.redis.Get(ctx, jobFloorKey).Result()
	if err == redis.Nil {
		return fallback, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get job floor: %w", err)
	}

	floor, err := strconv.Atoi(value)
	if err != nil {
		return fallback, nil
	}

	return floor, nil
}

// setJobFloor caches floor as the lowest disc count worth querying for a job, expiring after
// jobFloorTTL so boards later added below it (e.g. by an import) aren't skipped forever.
func (s *Server) setJobFloor(ctx context.Context, floor int) error {
	if err := s.redis.Set(ctx, jobFloorKey, floor, jobFloorTTL).Err(); err != nil {
		return fmt.Errorf("failed to set job floor: %w", err)
	}
	return nil
}

// recordJobCompletion increments workerID's positions-computed counter.
func (s *Server) recordJobCompletion(ctx context.Context, workerID string) error {
	pipe := s.redis.TxPipeline()
	pipe.HIncrBy(ctx, workerKey(workerID), workerFieldPositionsComputed, 1)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to record job completion: %w", err)
	}
	return nil
}

// heartbeat records workerID as active and refreshes its claim on board, if it reports one. The
// GET-compare-EXPIRE is not atomic: the claim could expire and be re-claimed between the two calls,
// extending the new owner's claim. Worst case is one duplicated evaluation, which is acceptable.
func (s *Server) heartbeat(ctx context.Context, workerID, hostname, gitCommit, board string) error {
	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, workerKey(workerID),
		workerFieldHostname, hostname,
		workerFieldGitCommit, gitCommit,
		workerFieldLastActive, time.Now().Format(time.RFC3339Nano),
	)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}

	if board == "" {
		return nil
	}

	owner, err := s.redis.Get(ctx, claimKey(board)).Result()
	if err == redis.Nil || (err == nil && owner != workerID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to refresh worker claim: %w", err)
	}

	if err := s.redis.Expire(ctx, claimKey(board), claimTTL).Err(); err != nil {
		return fmt.Errorf("failed to refresh worker claim: %w", err)
	}

	return nil
}

// workerInfo describes one worker, as listed by GET /api/workers.
type workerInfo struct {
	ID                string
	Hostname          string
	GitCommit         string
	PositionsComputed int
	LastActive        time.Time
}

// listWorkers returns active workers ordered by positions computed (most first), ties broken by last-active.
func (s *Server) listWorkers(ctx context.Context) ([]workerInfo, error) {
	var workers []workerInfo

	iter := s.redis.Scan(ctx, 0, "worker:*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		values, err := s.redis.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to get worker info: %w", err)
		}
		if len(values) == 0 {
			// Expired between SCAN and HGETALL.
			continue
		}

		positionsComputed, _ := strconv.Atoi(values[workerFieldPositionsComputed])
		lastActive, _ := time.Parse(time.RFC3339Nano, values[workerFieldLastActive])

		workers = append(workers, workerInfo{
			ID:                key[len("worker:"):],
			Hostname:          values[workerFieldHostname],
			GitCommit:         values[workerFieldGitCommit],
			PositionsComputed: positionsComputed,
			LastActive:        lastActive,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan workers: %w", err)
	}

	sort.Slice(workers, func(i, j int) bool {
		if workers[i].PositionsComputed != workers[j].PositionsComputed {
			return workers[i].PositionsComputed > workers[j].PositionsComputed
		}
		return workers[i].LastActive.After(workers[j].LastActive)
	})

	return workers, nil
}

// getCachedStats returns the cached stats JSON bytes from Redis, or (nil, nil) on a cache miss.
// Redis errors other than "key not found" are returned so callers can decide whether to fall through.
func (s *Server) getCachedStats(ctx context.Context) ([]byte, error) {
	data, err := s.redis.Get(ctx, statsKey).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cached stats: %w", err)
	}
	return data, nil
}

// setCachedStats stores pre-serialised stats JSON in Redis for statsTTL.
func (s *Server) setCachedStats(ctx context.Context, data []byte) {
	_ = s.redis.Set(ctx, statsKey, data, statsTTL).Err()
}

// invalidateStatsCache deletes the cached stats so the next GET /api/stats re-queries the DB.
func (s *Server) invalidateStatsCache(ctx context.Context) {
	_ = s.redis.Del(ctx, statsKey).Err()
}
