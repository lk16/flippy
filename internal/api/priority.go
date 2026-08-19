package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

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

// analysisResultTTL is how long a priority-computed evaluation lives in the ephemeral cache. Long enough
// for a polling loop to pick it up; short enough not to matter for boards that are never fetched.
const analysisResultTTL = 30 * time.Minute

// priorityClaimKey is the Redis key marking a claimed job as priority-originated, set alongside the
// normal claim key. handleSubmitJobResult checks it to decide whether to persist or skip DB writes for
// book-ineligible boards.
func priorityClaimKey(board string) string {
	return "priority_claim:" + board
}

// analysisResultKey is the Redis key holding a priority-computed evaluation as JSON, readable by
// lookupEvaluation as a third fallback after DB and minimax cache.
func analysisResultKey(board string) string {
	return "analysis:" + board
}

// enqueuePriority adds board to the priority queue at the given level if it is not already pending,
// skipping duplicates via the pending set rather than scanning the list. The pending set is keyed
// by board string only, so a board already pending at a lower level is not re-queued (the frontend
// will request the next level once the current job completes), and a board queued by a connection
// that has since died is not re-tagged for a live one — if the first requester disconnects, the
// entry is dropped at dequeue and a still-interested client re-queues it on its next request.
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

// dequeuePriority pops the oldest entry from the priority queue (FIFO), removing it from the
// pending set; ok is false when the queue is empty. Entries whose requesting connection has closed
// are discarded: nobody is waiting for the result. Entries that are plain board strings (legacy
// format) are treated as PriorityLevel with no connection.
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
			// Legacy format: plain board string.
			entry = priorityEntry{Board: data, Level: PriorityLevel}
		}

		_ = s.redis.SRem(ctx, priorityPendingKey, entry.Board).Err()

		if entry.ConnID != "" && !s.connLive(entry.ConnID) {
			continue
		}

		return entry, true, nil
	}
}

// setPriorityClaim marks board as priority-originated by setting a short-lived Redis key alongside the
// normal claim key. handleSubmitJobResult reads this to branch into the priority save path.
func (s *Server) setPriorityClaim(ctx context.Context, board string) error {
	if err := s.redis.Set(ctx, priorityClaimKey(board), "1", claimTTL).Err(); err != nil {
		return fmt.Errorf("failed to set priority claim: %w", err)
	}
	return nil
}

// consumePriorityClaim atomically reads and deletes the priority claim marker for board, reporting
// whether one existed. A missing key means the job originated from the normal ListLearnable path.
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

// setAnalysisResult stores eval in the ephemeral analysis cache for the given normalized board string.
// Best-effort: errors are silently discarded so a Redis hiccup never fails the job submission path.
func (s *Server) setAnalysisResult(ctx context.Context, board string, eval evaluationResponse) {
	data, err := json.Marshal(eval)
	if err != nil {
		return
	}
	_ = s.redis.Set(ctx, analysisResultKey(board), data, analysisResultTTL).Err()
}

// getAnalysisResult retrieves a priority-computed evaluation from the ephemeral cache.
// Returns (_, false, nil) on a cache miss; returns an error only for unexpected Redis failures.
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
