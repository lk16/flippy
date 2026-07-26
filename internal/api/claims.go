package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// claimTTL is how long a job claim (and the worker heartbeat backing it)
// survives without being refreshed. A worker that goes silent for longer
// than this loses its claim, and the job becomes claimable again.
const claimTTL = 5 * time.Minute

// claimKey is the redis key holding the worker ID that currently holds
// board's job, if any.
func claimKey(board string) string {
	return "claim:" + board
}

// workerKey is the redis key holding the board string a worker currently
// has claimed, if any. It lets heartbeat find what to refresh from just a
// worker ID.
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

	if err := s.redis.Set(ctx, workerKey(workerID), board, claimTTL).Err(); err != nil {
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
	if err := s.redis.Del(ctx, workerKey(workerID)).Err(); err != nil {
		return fmt.Errorf("failed to release worker claim: %w", err)
	}
	return nil
}

// heartbeat refreshes the TTL of workerID's current claim, if it has one.
// A worker with no active claim (idle between jobs) is not an error: there
// is simply nothing to refresh.
func (s *Server) heartbeat(ctx context.Context, workerID string) error {
	board, err := s.redis.Get(ctx, workerKey(workerID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to look up worker claim: %w", err)
	}

	pipe := s.redis.TxPipeline()
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	pipe.Expire(ctx, claimKey(board), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to refresh claim: %w", err)
	}

	return nil
}
