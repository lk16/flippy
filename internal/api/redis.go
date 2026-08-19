package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/edax"
)

// NewRedisClient opens a redis client for url and verifies it's reachable.
func NewRedisClient(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}

// claimTTL is how long a job claim or worker hash survives without a refresh.
const claimTTL = 5 * time.Minute

// bookStatsKey is the Redis hash of position counts per "<depth>:<confidence>:<discs>" field,
// rebuilt periodically because the underlying GROUP BY is slow at millions of rows.
const bookStatsKey = "book_stats"

// bookStatsRefreshInterval is how often the book_stats hash is rebuilt; both consumers tolerate
// the staleness.
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

// releaseClaim deletes the claim on board only if it still belongs to workerID, so a late release
// can't revoke a claim another worker took over. GET-then-DEL is not atomic; the worst case of the
// race is one duplicated evaluation, which is acceptable.
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
		fields = append(fields, fmt.Sprintf("%d:%d:%d", e.Depth, e.Confidence, e.DiscCount), e.Count)
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

// jobFloor returns the lowest disc count the book_stats hash still shows learnable positions for
// (search params below target; depth 0 counts), falling back to book.LeafDiscs when the hash is
// missing. May be stale by one refresh; it is only a lower bound for the ListLearnable scan.
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

// heartbeat records workerID as active and refreshes its claim on board ("" for none), but never
// another worker's claim. GET-compare-EXPIRE is not atomic; the worst case of the race is one
// duplicated evaluation, which is acceptable.
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

// priorityEntry is the value stored in the priority queue for one pending evaluation job. ConnID is
// the websocket connection that requested it ("" for none); entries whose connection has since
// closed are discarded at dequeue.
type priorityEntry struct {
	Board  string `json:"board"`
	Level  int    `json:"level"`
	ConnID string `json:"conn_id,omitempty"`
}

// priorityQueueKey is a Redis list: boards waiting for priority evaluation (LPUSH to enqueue, RPOP to drain).
const priorityQueueKey = "priority_jobs"

// priorityPendingKey is a Redis set mirroring priorityQueueKey's contents for O(1) duplicate detection.
const priorityPendingKey = "priority_pending"

// analysisResultTTL is how long a priority-computed evaluation lives in the ephemeral cache: long
// enough for the frontend's polling loop to pick it up.
const analysisResultTTL = 30 * time.Minute

// priorityClaimKey is the Redis key marking a claimed job as priority-originated.
func priorityClaimKey(board string) string {
	return "priority_claim:" + board
}

// analysisResultKey is the Redis key holding a priority-computed evaluation as JSON.
func analysisResultKey(board string) string {
	return "analysis:" + board
}

// enqueuePriority adds board to the priority queue unless already pending. Dedupe is by board
// alone: a board pending at another level or for another (possibly dead) connection is not
// re-queued — the frontend re-requests until it gets a result, so drops heal themselves.
func (s *Server) enqueuePriority(ctx context.Context, board string, level int, connID string) error {
	isMember, err := s.redis.SIsMember(ctx, priorityPendingKey, board).Result()
	if err != nil {
		return fmt.Errorf("failed to check priority pending set: %w", err)
	}
	if isMember {
		return nil
	}

	data, err := json.Marshal(priorityEntry{Board: board, Level: level, ConnID: connID})
	if err != nil {
		return fmt.Errorf("failed to marshal priority entry: %w", err)
	}

	pipe := s.redis.TxPipeline()
	pipe.SAdd(ctx, priorityPendingKey, board)
	pipe.LPush(ctx, priorityQueueKey, string(data))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to enqueue priority board: %w", err)
	}
	return nil
}

// dequeuePriority pops the oldest entry (FIFO), removing it from the pending set; ok is false when
// the queue is empty. Entries whose requesting connection has closed are discarded: nobody is
// waiting for the result.
func (s *Server) dequeuePriority(ctx context.Context) (priorityEntry, bool, error) {
	for {
		data, err := s.redis.RPop(ctx, priorityQueueKey).Result()
		if err == redis.Nil {
			return priorityEntry{}, false, nil
		}
		if err != nil {
			return priorityEntry{}, false, fmt.Errorf("failed to dequeue priority board: %w", err)
		}

		var entry priorityEntry
		if jsonErr := json.Unmarshal([]byte(data), &entry); jsonErr != nil {
			// Legacy entries are plain board strings.
			entry = priorityEntry{Board: data, Level: PriorityLevel}
		}

		_ = s.redis.SRem(ctx, priorityPendingKey, entry.Board).Err()

		if entry.ConnID != "" && !s.connLive(entry.ConnID) {
			continue
		}

		return entry, true, nil
	}
}

// setPriorityClaim marks board's claim as priority-originated.
func (s *Server) setPriorityClaim(ctx context.Context, board string) error {
	if err := s.redis.Set(ctx, priorityClaimKey(board), "1", claimTTL).Err(); err != nil {
		return fmt.Errorf("failed to set priority claim: %w", err)
	}
	return nil
}

// consumePriorityClaim atomically reads and deletes board's priority claim marker, reporting
// whether one existed.
func (s *Server) consumePriorityClaim(ctx context.Context, board string) (bool, error) {
	val, err := s.redis.GetDel(ctx, priorityClaimKey(board)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to consume priority claim: %w", err)
	}
	return val != "", nil
}

// setAnalysisResult stores eval in the ephemeral analysis cache; best-effort, so a Redis hiccup
// never fails the submission path.
func (s *Server) setAnalysisResult(ctx context.Context, board string, eval evaluationResponse) {
	data, err := json.Marshal(eval)
	if err != nil {
		return
	}
	_ = s.redis.Set(ctx, analysisResultKey(board), data, analysisResultTTL).Err()
}

// getAnalysisResult retrieves a priority-computed evaluation from the ephemeral cache; ok is false
// on a miss.
func (s *Server) getAnalysisResult(ctx context.Context, board string) (evaluationResponse, bool, error) {
	data, err := s.redis.Get(ctx, analysisResultKey(board)).Bytes()
	if err == redis.Nil {
		return evaluationResponse{}, false, nil
	}
	if err != nil {
		return evaluationResponse{}, false, fmt.Errorf("failed to get analysis result: %w", err)
	}

	var eval evaluationResponse
	if err := json.Unmarshal(data, &eval); err != nil {
		return evaluationResponse{}, false, fmt.Errorf("failed to unmarshal analysis result: %w", err)
	}
	return eval, true, nil
}
