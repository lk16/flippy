package api

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/edax"
)

// claimTTL is how long a job claim or worker hash survives without a refresh.
const claimTTL = 5 * time.Minute

// bookStatsKey is the Redis hash holding position counts per "<depth>:<confidence>:<discs>" field,
// rebuilt periodically from the DB (see RunBookStatsRefresh). GET /api/stats serves it, and the job
// floor is derived from it, since the underlying GROUP BY scans every row in the boards table and
// becomes slow at millions of rows.
const bookStatsKey = "book_stats"

// bookStatsRefreshInterval is how often the book_stats hash is rebuilt from the DB. Both consumers
// tolerate the staleness: stats are informational, and the job floor is only a lower bound for the
// ListLearnable scan.
const bookStatsRefreshInterval = 60 * time.Second

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

// RunBookStatsRefresh rebuilds the book_stats hash immediately and then every
// bookStatsRefreshInterval, until ctx is canceled.
func (s *Server) RunBookStatsRefresh(ctx context.Context) {
	if err := s.rebuildBookStats(ctx); err != nil {
		log.Printf("failed to rebuild book stats: %v", err)
	}

	ticker := time.NewTicker(bookStatsRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.rebuildBookStats(ctx); err != nil {
				log.Printf("failed to rebuild book stats: %v", err)
			}
		}
	}
}

// bookStatsField formats a book_stats hash field name.
func bookStatsField(e statEntry) string {
	return fmt.Sprintf("%d:%d:%d", e.Depth, e.Confidence, e.DiscCount)
}

// rebuildBookStats queries the DB and replaces the book_stats hash, writing into a temp key and
// atomically swapping it in with RENAME so readers never see a partial hash.
func (s *Server) rebuildBookStats(ctx context.Context) error {
	stats, err := s.repo.Stats(ctx)
	if err != nil {
		return fmt.Errorf("failed to query stats: %w", err)
	}

	entries := statEntries(stats)
	if len(entries) == 0 {
		// RENAME fails on a missing source key; an absent hash already means "fall back to the DB".
		if err := s.redis.Del(ctx, bookStatsKey).Err(); err != nil {
			return fmt.Errorf("failed to clear book stats: %w", err)
		}
		return nil
	}

	fields := make([]any, 0, 2*len(entries))
	for _, e := range entries {
		fields = append(fields, bookStatsField(e), e.Count)
	}

	tempKey := bookStatsKey + ":rebuild"
	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, tempKey)
	pipe.HSet(ctx, tempKey, fields...)
	pipe.Rename(ctx, tempKey, bookStatsKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to rebuild book stats: %w", err)
	}

	return nil
}

// getBookStats reads the book_stats hash, sorted like statEntries; ok is false when the hash is
// missing (Redis flushed, first boot race) and the caller should fall back to the DB.
func (s *Server) getBookStats(ctx context.Context) (entries []statEntry, ok bool, err error) {
	values, err := s.redis.HGetAll(ctx, bookStatsKey).Result()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get book stats: %w", err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}

	entries = make([]statEntry, 0, len(values))
	for field, value := range values {
		var e statEntry
		if _, err := fmt.Sscanf(field, "%d:%d:%d", &e.Depth, &e.Confidence, &e.DiscCount); err != nil {
			continue
		}
		count, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		e.Count = count
		entries = append(entries, e)
	}

	sortStatEntries(entries)
	return entries, true, nil
}

// jobFloor returns the lowest disc count that still has learnable positions according to the
// book_stats hash: a field whose (depth, confidence) is below what a target-level search requires
// (depth 0, an unlearned board, counts as learnable). Falls back to book.LeafDiscs when the hash is
// missing. The result may be stale by up to bookStatsRefreshInterval, which is fine: it is only a
// lower bound for the ListLearnable scan.
func (s *Server) jobFloor(ctx context.Context) int {
	entries, ok, err := s.getBookStats(ctx)
	if err != nil || !ok {
		return book.LeafDiscs
	}

	floor := book.MaxSavableDiscs
	found := false
	for _, e := range entries {
		if e.DiscCount < book.LeafDiscs || e.DiscCount > book.MaxSavableDiscs {
			continue
		}
		targetDepth, targetConfidence := edax.SearchParams(e.DiscCount, TargetLevel(e.DiscCount))
		learnable := e.Depth == 0 ||
			e.Depth < targetDepth ||
			(e.Depth == targetDepth && e.Confidence < targetConfidence)
		if learnable && (!found || e.DiscCount < floor) {
			floor = e.DiscCount
			found = true
		}
	}

	return floor
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
