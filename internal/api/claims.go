package api

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// claimTTL is how long a job claim, and the worker hash backing it, survive
// without being refreshed. A worker that goes silent for longer than this
// loses its claim (which becomes claimable again) and disappears from the
// worker listing.
const claimTTL = 5 * time.Minute

// Redis field names within a worker's hash (see workerKey).
const (
	workerFieldHostname          = "hostname"
	workerFieldGitCommit         = "git_commit"
	workerFieldPositionsComputed = "positions_computed"
	workerFieldLastActive        = "last_active"
	workerFieldClaimedBoard      = "claimed_board"
)

// claimKey is the redis key holding the worker ID that currently holds
// board's job, if any.
func claimKey(board string) string {
	return "claim:" + board
}

// workerKey is the redis key of the hash describing a worker: hostname,
// git commit, positions computed, last active time, and the board it
// currently has claimed (if any). It exists once a worker has sent at
// least one heartbeat or claimed at least one job.
func workerKey(workerID string) string {
	return "worker:" + workerID
}

// tryClaim attempts to atomically claim board for workerID. It reports
// false, with no error, if board is already claimed by someone else.
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
		workerFieldClaimedBoard, board,
		workerFieldLastActive, time.Now().Format(time.RFC3339Nano),
	)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("failed to record worker claim: %w", err)
	}

	return true, nil
}

// releaseClaim releases workerID's claim on board, if any. It's best-effort:
// claims also expire on their own via claimTTL, so a failure to release
// here only delays a job becoming reclaimable, it doesn't lose anything.
func (s *Server) releaseClaim(ctx context.Context, board, workerID string) error {
	if err := s.redis.Del(ctx, claimKey(board)).Err(); err != nil {
		return fmt.Errorf("failed to release claim: %w", err)
	}

	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, workerKey(workerID), workerFieldClaimedBoard, "")
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear worker claim: %w", err)
	}

	return nil
}

// recordJobCompletion increments workerID's positions-computed counter.
// It's called once a job result has been saved successfully.
func (s *Server) recordJobCompletion(ctx context.Context, workerID string) error {
	pipe := s.redis.TxPipeline()
	pipe.HIncrBy(ctx, workerKey(workerID), workerFieldPositionsComputed, 1)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to record job completion: %w", err)
	}
	return nil
}

// heartbeat records workerID as active (hostname, git commit, last active
// time) and refreshes its claim, if it has one, so a long-running
// evaluation doesn't lose it between heartbeats.
func (s *Server) heartbeat(ctx context.Context, workerID, hostname, gitCommit string) error {
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

	board, err := s.redis.HGet(ctx, workerKey(workerID), workerFieldClaimedBoard).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to look up worker claim: %w", err)
	}
	if board == "" {
		return nil
	}

	if err := s.redis.Expire(ctx, claimKey(board), claimTTL).Err(); err != nil {
		return fmt.Errorf("failed to refresh claim: %w", err)
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

// listWorkers returns every worker with an active (non-expired) hash,
// ordered by positions computed (most first); ties (e.g. two workers still
// on their first job) break by most-recently-active. Sorting by last-active
// alone would reorder the list on almost every poll, since heartbeats and
// job completions land in a slightly different order each time.
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
