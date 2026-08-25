package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

func TestNewRedisClient_InvalidURL(t *testing.T) {
	_, err := NewRedisClient(context.Background(), "not-a-valid-url")
	require.Error(t, err)
}

func TestNewRedisClient_Success(t *testing.T) {
	url := os.Getenv("FLIPPY_REDIS_URL")
	if url == "" {
		t.Skip("FLIPPY_REDIS_URL not set; skipping test requiring redis")
	}

	client, err := NewRedisClient(context.Background(), url)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
}

func TestServer_TryClaim_SecondClaimFails(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = s.tryClaim(ctx, "position-a", "worker-2")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestServer_TryClaim_DistinctBoardsBothSucceed(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = s.tryClaim(ctx, "position-b", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestServer_ReleaseClaim_AllowsReclaim(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.releaseClaim(ctx, "position-a", "worker-1"))

	ok, err = s.tryClaim(ctx, "position-a", "worker-2")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestServer_ReleaseClaim_NoActiveClaimIsNoop(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.releaseClaim(context.Background(), "position-a", "worker-1"))
}

// TestServer_ReleaseClaim_DoesNotRevokeAnotherWorkersClaim covers the race
// where worker-1's claim TTL expired and worker-2 re-claimed the same position:
// worker-1 finishing late and releasing must not delete worker-2's claim.
func TestServer_ReleaseClaim_DoesNotRevokeAnotherWorkersClaim(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// worker-1 claims, then its claim is taken over by worker-2 (simulating the
	// original TTL having expired and worker-2 winning the re-claim).
	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.redis.Set(ctx, claimKey("position-a"), "worker-2", claimTTL).Err())

	// worker-1's late release must leave worker-2's claim intact.
	require.NoError(t, s.releaseClaim(ctx, "position-a", "worker-1"))

	owner, err := s.redis.Get(ctx, claimKey("position-a")).Result()
	require.NoError(t, err)
	require.Equal(t, "worker-2", owner)
}

func TestServer_RebuildBookStats_ProducesExpectedFields(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// One 12-disc position learned at level 20 (a 20@73% search), one unlearned (depth 0, confidence 0).
	positions := testDistinctPositions(t, 12, 2)
	require.NoError(t, s.repo.AddPositions(ctx, positions))
	require.NoError(t, s.repo.SaveEvaluation(ctx, positions[0], db.Evaluation{Level: 20, Score: 2}))

	require.NoError(t, s.rebuildBookStats(ctx))

	values, err := s.redis.HGetAll(ctx, bookStatsKey).Result()
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"0:0:12":   "1",
		"20:73:12": "1",
	}, values)
}

func TestServer_RebuildBookStats_EmptyDBClearsHash(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.redis.HSet(ctx, bookStatsKey, "0:0:12", 1).Err())

	require.NoError(t, s.rebuildBookStats(ctx))

	_, ok, err := s.getBookStats(ctx)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestServer_BookStats_IncrementalSave(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.rebuildBookStats(ctx))

	// A submitted target-level result moves the position's counter to the new cell without a
	// rebuild; the decremented-to-zero unlearned cell is filtered out of reads.
	level := TargetLevel(12)
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", jobResultRequest{
		WorkerID: "w1", Position: position.String(), Level: level, Score: 2,
	})
	require.Equal(t, http.StatusOK, w.Code)

	depth, confidence := edax.SearchParams(12, level)
	entries, ok, err := s.getBookStats(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []statEntry{classifiedStat(12, depth, confidence, 1)}, entries)
}

func TestServer_BookStats_NoPartialHashCreated(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	// No hash exists yet; an incremental update must not create a nearly-empty one.
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", jobResultRequest{
		WorkerID: "w1", Position: position.String(), Level: TargetLevel(12), Score: 2,
	})
	require.Equal(t, http.StatusOK, w.Code)

	exists, err := s.redis.Exists(ctx, bookStatsKey).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}

func TestServer_GetBookStats_MissingHash(t *testing.T) {
	s := testServer(t)

	_, ok, err := s.getBookStats(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestServer_GetBookStats_ReturnsSortedEntries(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.redis.HSet(ctx, bookStatsKey,
		"20:73:13", 3,
		"0:0:12", 7,
		"20:73:12", 2,
	).Err())

	entries, ok, err := s.getBookStats(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []statEntry{
		classifiedStat(12, 0, 0, 7),
		classifiedStat(12, 20, 73, 2),
		classifiedStat(13, 20, 73, 3),
	}, entries)
}

func TestServer_JobFloor_FallsBackWhenHashMissing(t *testing.T) {
	s := testServer(t)
	require.Equal(t, book.LeafDiscs, s.jobFloor(context.Background()))
}

func TestServer_JobFloor_PicksLowestLearnableDiscCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// 12-disc position fully learned, 13-disc position unlearned: the floor advances to 13.
	position12 := testPosition(t, 12)
	position13 := testPosition(t, 13)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position12, position13}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position12, db.Evaluation{Level: TargetLevel(12), Score: 0}))
	require.NoError(t, s.rebuildBookStats(ctx))

	require.Equal(t, 13, s.jobFloor(ctx))
}

func TestServer_JobFloor_BelowTargetSearchCountsAsLearnable(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Learned, but below the target level: still learnable, so the floor stays at 12.
	position12 := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position12}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position12, db.Evaluation{Level: 20, Score: 0}))
	require.NoError(t, s.rebuildBookStats(ctx))

	require.Equal(t, 12, s.jobFloor(ctx))
}

func TestServer_JobFloor_IgnoresDiscCountsOutsideSavableRange(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// The hash is written directly: AddPositions refuses out-of-range rows, but an older book's
	// leftovers can still show up in a resync, and they must not drag the floor to a disc count no
	// job is ever handed out for.
	require.NoError(t, s.redis.HSet(ctx, bookStatsKey, "0:0:35", 4, "0:0:5", 2).Err())

	require.Equal(t, book.MaxSavableDiscs, s.jobFloor(ctx))
}

func TestServer_RecordJobCompletion_IncrementsCounter(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.recordJobCompletion(ctx, "worker-1"))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-1"))

	count, err := s.redis.HGet(ctx, workerKey("worker-1"), workerFieldPositionsComputed).Result()
	require.NoError(t, err)
	require.Equal(t, "2", count)
}

func TestServer_Heartbeat_RefreshesClaimTTL(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.redis.Expire(ctx, claimKey("position-a"), time.Second).Err())

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", "position-a"))

	ttl, err := s.redis.TTL(ctx, claimKey("position-a")).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Second)
}

// TestServer_Heartbeat_DoesNotRefreshClaimTakenOverByAnotherWorker covers a
// position whose claim expired and was re-claimed by another worker: the first
// worker's heartbeat must not extend the new owner's claim.
func TestServer_Heartbeat_DoesNotRefreshClaimTakenOverByAnotherWorker(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	// worker-2 takes over the position (as if worker-1's TTL had lapsed).
	require.NoError(t, s.redis.Set(ctx, claimKey("position-a"), "worker-2", time.Second).Err())

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", "position-a"))

	// worker-2's short TTL must be left untouched.
	ttl, err := s.redis.TTL(ctx, claimKey("position-a")).Result()
	require.NoError(t, err)
	require.LessOrEqual(t, ttl, time.Second)
}

func TestServer_Heartbeat_RecordsHostnameAndGitCommit(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", ""))

	values, err := s.redis.HGetAll(ctx, workerKey("worker-1")).Result()
	require.NoError(t, err)
	require.Equal(t, "host-1", values[workerFieldHostname])
	require.Equal(t, "commit-1", values[workerFieldGitCommit])
	require.NotEmpty(t, values[workerFieldLastActive])
}

func TestServer_Heartbeat_IdleWorkerIsNotAnError(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "worker-unknown", "host-1", "commit-1", ""))
}

func TestServer_Heartbeat_UnclaimedPositionIsNotAnError(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "worker-1", "host-1", "commit-1", "position-a"))
}

func TestServer_ListWorkers_Empty(t *testing.T) {
	s := testServer(t)

	workers, err := s.listWorkers(context.Background())
	require.NoError(t, err)
	require.Empty(t, workers)
}

func TestServer_ListWorkers_OrdersByPositionsComputedDescending(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", ""))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-1"))

	require.NoError(t, s.heartbeat(ctx, "worker-2", "host-2", "commit-2", ""))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-2"))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-2"))

	workers, err := s.listWorkers(ctx)
	require.NoError(t, err)
	require.Len(t, workers, 2)
	require.Equal(t, "worker-2", workers[0].ID)
	require.Equal(t, "worker-1", workers[1].ID)
}

func TestServer_ListWorkers_TiesBreakByMostRecentlyActive(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", ""))
	require.NoError(t, s.heartbeat(ctx, "worker-2", "host-2", "commit-2", ""))

	workers, err := s.listWorkers(ctx)
	require.NoError(t, err)
	require.Len(t, workers, 2)
	require.Equal(t, "worker-2", workers[0].ID)
	require.Equal(t, "worker-1", workers[1].ID)
}

func TestServer_ListWorkers_IncludesClaimAndPositionsComputed(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", ""))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-1"))

	// An active claim key must not confuse listWorkers' SCAN over "worker:*".
	ok, err := s.tryClaim(ctx, "position-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	workers, err := s.listWorkers(ctx)
	require.NoError(t, err)
	require.Len(t, workers, 1)
	require.Equal(t, "worker-1", workers[0].ID)
	require.Equal(t, "host-1", workers[0].Hostname)
	require.Equal(t, "commit-1", workers[0].GitCommit)
	require.Equal(t, 1, workers[0].PositionsComputed)
	require.False(t, workers[0].LastActive.IsZero())
}

// drainPriority pops every entry currently in the priority queue.
func drainPriority(t *testing.T, s *Server) []priorityEntry {
	t.Helper()

	var entries []priorityEntry
	for {
		entry, ok, err := s.dequeuePriority(context.Background())
		require.NoError(t, err)
		if !ok {
			return entries
		}
		entries = append(entries, entry)
	}
}

// TestClaimJob_PriorityDrainedFirst verifies that priority-queue positions are returned before
// any book candidates.
func TestClaimJob_PriorityDrainedFirst(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Put a learnable position in the DB so the buffer has something to offer.
	dbBoard := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{dbBoard}))

	// Enqueue a different position as priority.
	pBoard := testPosition(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel, ""))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, pBoard, job.Position)
	require.Equal(t, PriorityLevel, job.Level)

	// The next claim falls back to the DB position.
	job, ok, err = s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, dbBoard, job.Position)
}

// TestClaimJob_PrioritySkipsNoMovesBoard ensures positions with no legal move are skipped in the priority path.
func TestClaimJob_PrioritySkipsNoMovesBoard(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Construct a no-legal-move position.
	var blackBits, whiteBits uint64
	for i := range uint(40) {
		blackBits |= 1 << i
	}
	for i := uint(40); i < 64; i++ {
		whiteBits |= 1 << i
	}
	noMovePosition, err := othello.NewPosition(blackBits, whiteBits)
	require.NoError(t, err)
	require.False(t, noMovePosition.HasMoves())
	normalizedNoMove := noMovePosition.Normalize()

	// Enqueue it in the priority queue (server shouldn't claim it).
	require.NoError(t, s.enqueuePriority(ctx, normalizedNoMove.String(), PriorityLevel, ""))

	// Add a normal DB position too.
	dbBoard := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{dbBoard}))

	// The no-move position is skipped; the claim falls through to the DB position.
	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, dbBoard, job.Position)
}

// TestClaimJob_PriorityDeduplicates verifies that enqueuePriority skips duplicates.
func TestClaimJob_PriorityDeduplicates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	pBoard := testPosition(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel, ""))
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel, "")) // duplicate — should be ignored

	require.Len(t, drainPriority(t, s), 1)
}

// TestDequeuePriority_PromotesEntriesFromDeadConnections verifies that an entry queued by a since-
// closed websocket connection leaves the queue and the pending set, but is added to the book as an
// unlearned row rather than thrown away: nobody is waiting for it, yet the work is still wanted.
func TestDequeuePriority_PromotesEntriesFromDeadConnections(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	connID, err := s.registerConn(ctx)
	require.NoError(t, err)
	position := testPosition(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, position.String(), PriorityLevel, connID))
	s.unregisterConn(ctx, connID)

	_, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	// Removed from the pending set too, so the position can be re-queued.
	isMember, err := s.redis.SIsMember(ctx, priorityPendingKey, position.String()).Result()
	require.NoError(t, err)
	require.False(t, isMember)

	// Now in the book, unlearned, so the second job tier hands it out.
	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval)

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position, job.Position)
	require.Equal(t, UnlearnedLevel(14), job.Level)
}

// TestDequeuePriority_DoesNotPromoteOutOfRangeDiscCounts covers the one case a dropped entry stays
// dropped: no row may exist for it, so there is nothing to promote it into.
func TestDequeuePriority_DoesNotPromoteOutOfRangeDiscCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	connID, err := s.registerConn(ctx)
	require.NoError(t, err)
	position := testPosition(t, book.MaxSavableDiscs+5)
	require.NoError(t, s.enqueuePriority(ctx, position.String(), PriorityLevel, connID))
	s.unregisterConn(ctx, connID)

	_, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	_, err = s.repo.GetPosition(ctx, position.Position())
	require.ErrorIs(t, err, db.ErrPositionNotFound)
}

// TestPromoteToBook_LeavesAnExistingRowAlone covers the other non-promotion: a position the book
// already holds needs no row, and its evaluation must survive the request that dropped.
func TestPromoteToBook_LeavesAnExistingRowAlone(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	saved := db.Evaluation{Level: TargetLevel(14), Score: 3}
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, saved))

	require.False(t, s.promoteToBook(ctx, position.String()))

	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, saved, eval)
}

// TestPromoteToBook_IgnoresUnusablePositions covers the entries a corrupted queue can hold.
func TestPromoteToBook_IgnoresUnusablePositions(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.False(t, s.promoteToBook(ctx, "not-a-position"))

	// Parseable but not normalized, so it would not match the claim key it was queued under.
	unnormalized, err := othello.NewStartPosition().DoMove(19)
	require.NoError(t, err)
	require.False(t, unnormalized.IsNormalized())
	require.False(t, s.promoteToBook(ctx, unnormalized.String()))
}

// TestDequeuePriority_KeepsEntriesFromLiveConnections verifies that entries whose connection is
// still open come through, including when they sit behind a dead connection's entry.
func TestDequeuePriority_KeepsEntriesFromLiveConnections(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	deadConn, err := s.registerConn(ctx)
	require.NoError(t, err)
	liveConn, err := s.registerConn(ctx)
	require.NoError(t, err)

	deadBoard := testPosition(t, 14)
	liveBoard := testPosition(t, 15)
	require.NoError(t, s.enqueuePriority(ctx, deadBoard.String(), PriorityLevel, deadConn))
	require.NoError(t, s.enqueuePriority(ctx, liveBoard.String(), PriorityLevel, liveConn))
	s.unregisterConn(ctx, deadConn)

	entry, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, liveBoard.String(), entry.Position)

	_, ok, err = s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)
}

// Dedupe keys on the position alone: an entry whose first requester died is dropped even if a live
// connection asked too; that client's next request re-queues it.
func TestDequeuePriority_DedupeIsByBoardOnly(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	connA, err := s.registerConn(ctx)
	require.NoError(t, err)
	connB, err := s.registerConn(ctx)
	require.NoError(t, err)
	position := testPosition(t, 14)

	require.NoError(t, s.enqueuePriority(ctx, position.String(), PriorityLevel, connA))
	require.NoError(t, s.enqueuePriority(ctx, position.String(), PriorityLevel, connB)) // deduped: still tagged connA
	s.unregisterConn(ctx, connA)

	_, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	// connB re-requesting after the drop queues the position again.
	require.NoError(t, s.enqueuePriority(ctx, position.String(), PriorityLevel, connB))
	entry, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position.String(), entry.Position)
}

// TestDequeuePriority_UntaggedEntriesAreNeverDropped covers entries with no connection ID (legacy
// format): they cannot be matched to a live connection, so they always come through.
func TestDequeuePriority_UntaggedEntriesAreNeverDropped(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, position.String(), PriorityLevel, ""))

	entry, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position.String(), entry.Position)
}

// TestHandleSubmitJobResult_PriorityHighDiscSkipsPersistence verifies that a priority job for
// a position with > MaxSavableDiscs does not attempt a DB write but does cache the result ephemerally.
func TestHandleSubmitJobResult_PriorityHighDiscSkipsPersistence(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Use a position with many discs to exceed MaxSavableDiscs (30).
	position := testPosition(t, 35)
	require.Greater(t, position.CountDiscs(), book.MaxSavableDiscs)

	require.Equal(t, 200, submitPriorityResult(t, s, position, TargetLevel(position.CountDiscs()), 2).Code)

	// Position should not have been inserted into boards table.
	_, err := s.repo.GetPosition(ctx, position.Position())
	require.Error(t, err)

	// But the ephemeral cache should have it.
	cached, ok, err := s.getAnalysisResult(ctx, position.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, cached.Score)
}

// submitPriorityResult claims position as a priority job and submits an evaluation at level for it,
// returning the response recorder.
func submitPriorityResult(t *testing.T, s *Server, position othello.NormalizedPosition, level, score int) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, s.setPriorityClaim(ctx, position.String()))
	claimed, err := s.tryClaim(ctx, position.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	depth, confidence := edax.SearchParams(position.CountDiscs(), level)
	return doRequest(t, s, "POST", "/api/jobs/result", jobResultRequest{
		WorkerID: "w1", Position: position.String(), Level: level,
		Depth: depth, Confidence: confidence, Score: score,
	})
}

// TestHandleSubmitJobResult_PriorityLowDiscPersists verifies that a priority job for a position
// with <= MaxSavableDiscs (which already has a row), searched at its target level, saves the
// evaluation to the DB.
func TestHandleSubmitJobResult_PriorityLowDiscPersists(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	target := TargetLevel(position.CountDiscs())
	require.Equal(t, 200, submitPriorityResult(t, s, position, target, 6).Code)

	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: target, Score: 6}, eval)
}

// TestHandleSubmitJobResult_PriorityLowDiscNoRowAddsAndSaves verifies the AddPositions+retry path:
// when a priority <=30-disc position searched at its target level has no existing row, one is created.
func TestHandleSubmitJobResult_PriorityLowDiscNoRowAddsAndSaves(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	// Do NOT call AddPositions; the position has no row.

	target := TargetLevel(position.CountDiscs())
	require.Equal(t, 200, submitPriorityResult(t, s, position, target, -4).Code)

	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: target, Score: -4}, eval)
}

// TestHandleSubmitJobResult_PriorityBelowFloorNotPersisted verifies the level floor: a priority
// result shallower than any job tier hands out leaves the existing row unevaluated, so a client
// asking for a token search never lands one in the book.
func TestHandleSubmitJobResult_PriorityBelowFloorNotPersisted(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	require.Equal(t, 200, submitPriorityResult(t, s, position, UnlearnedLevel(14)-1, 6).Code)

	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval, "row stays unevaluated")

	// The result is still served back: it lives in the ephemeral analysis cache.
	cached, ok, err := s.getAnalysisResult(ctx, position.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 6, cached.Score)
}

// TestHandleSubmitJobResult_PriorityBelowFloorSchedulesBoardForLearning verifies that a priority
// result too shallow to keep, on an unknown savable position, still creates a row with an empty
// evaluation, so the unlearned scan picks the position up later — without seeding the book with the
// shallow score.
func TestHandleSubmitJobResult_PriorityBelowFloorSchedulesBoardForLearning(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	// Do NOT call AddPositions; the position has no row.

	require.Equal(t, 200, submitPriorityResult(t, s, position, UnlearnedLevel(14)-1, 6).Code)

	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval, "row exists but stays unevaluated")
}

// TestHandleSubmitJobResult_PriorityBelowTargetDoesNotDowngradeExistingRow verifies the
// insert-if-absent guarantee: a position with a real evaluation is untouched by a shallow result.
func TestHandleSubmitJobResult_PriorityBelowTargetDoesNotDowngradeExistingRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 14)
	target := TargetLevel(position.CountDiscs())
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: target, Score: 5}))

	require.Equal(t, 200, submitPriorityResult(t, s, position, UnlearnedLevel(14), 6).Code)

	eval, err := s.repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: target, Score: 5}, eval)
}

// TestHandleSubmitJobResult_PriorityBelowTargetHighDiscAddsNoRow verifies that positions above
// MaxSavableDiscs are not scheduled for learning either.
func TestHandleSubmitJobResult_PriorityBelowTargetHighDiscAddsNoRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 35)
	require.Greater(t, position.CountDiscs(), book.MaxSavableDiscs)

	require.Equal(t, 200, submitPriorityResult(t, s, position, PriorityLevel, 6).Code)

	_, err := s.repo.GetPosition(ctx, position.Position())
	require.ErrorIs(t, err, db.ErrPositionNotFound)
}

// TestHandleSubmitJobResult_NonPriorityBoardNotFoundStill404 ensures the existing 404 path
// for non-priority jobs is untouched by the priority changes.
func TestHandleSubmitJobResult_NonPriorityBoardNotFoundStill404(t *testing.T) {
	s := testServer(t)
	position := testPosition(t, 12)
	// No priority claim, no DB row.
	reqBody := jobResultRequest{WorkerID: "w1", Position: position.String(), Level: TargetLevel(12), Score: 0}
	w := doRequest(t, s, "POST", "/api/jobs/result", reqBody)
	require.Equal(t, 404, w.Code)
}

// TestLookupEvaluation_EphemeralCacheFallback verifies that lookupEvaluation returns an
// analysis result stored in the ephemeral cache when the DB and minimax cache both miss.
func TestLookupEvaluation_EphemeralCacheFallback(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 20)
	eval := evaluationResponse{Level: PriorityLevel, Depth: PriorityLevel, Confidence: 100, Score: 8, Source: evaluationSourceEdax}
	s.setAnalysisResult(ctx, position.String(), eval)

	result, ok, err := s.lookupEvaluation(ctx, position.Position())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, eval, result)
}

func TestConnLiveness_RedisBacked(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	connID, err := s.registerConn(ctx)
	require.NoError(t, err)
	require.True(t, s.connLive(ctx, connID))

	// The key carries a TTL, so a crashed replica's connections expire on their own.
	ttl, err := s.redis.TTL(ctx, connKey(connID)).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Duration(0))

	s.unregisterConn(ctx, connID)
	require.False(t, s.connLive(ctx, connID))
}

func TestRefreshBookStatsIfElected(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	// First election wins the lock and rebuilds the hash.
	s.refreshBookStatsIfElected(ctx)
	_, ok, err := s.getBookStats(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	owner, err := s.redis.Get(ctx, bookStatsLockKey).Result()
	require.NoError(t, err)
	require.Equal(t, s.replicaID, owner)

	// While the lock is held, a tick on any replica refreshes nothing.
	require.NoError(t, s.redis.Del(ctx, bookStatsKey).Err())
	s.refreshBookStatsIfElected(ctx)
	_, ok, err = s.getBookStats(ctx)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestJobCursorEncoding_RoundTrip(t *testing.T) {
	position := testPosition(t, 12)
	cursor := db.LearnableCursor{DiscCount: 12, Level: 24, Position: position}

	require.Equal(t, cursor, decodeJobCursor(encodeJobCursor(cursor)))
}

// TestDecodeJobCursor_MalformedStartsOver pins the fallback the sweep relies on: a cursor Redis
// can't return intact costs a rescan, not a failed claim.
func TestDecodeJobCursor_MalformedStartsOver(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{"empty", ""},
		{"too few fields", "12:24"},
		{"disc count not a number", "x:24:" + testPosition(t, 12).String()},
		{"level not a number", "12:x:" + testPosition(t, 12).String()},
		{"position unparseable", "12:24:not-a-position"},
		{"position not normalized", "12:24:" + testNonNormalizedPosition(t).String()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, db.LearnableCursor{}, decodeJobCursor(test.encoded))
		})
	}
}

// testPartiallyLearnedPosition returns a position with a row searched below its target level, so
// the sweep in refillFromPartiallyLearned is the only tier that offers it.
func testPartiallyLearnedPosition(t *testing.T, s *Server, discs int) othello.NormalizedPosition {
	t.Helper()
	ctx := context.Background()

	position := testPosition(t, discs)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: UnlearnedLevel(discs), Score: 0}))

	return position
}

// bufferedPositions returns the shared buffer's contents, oldest first.
func bufferedPositions(t *testing.T, s *Server) []string {
	t.Helper()

	positions, err := s.redis.LRange(context.Background(), jobBufferKey, 0, -1).Result()
	require.NoError(t, err)
	return positions
}

// TestServer_RefillJobBuffer_BuffersUnlearnedBeforePartiallyLearned covers the tier order between
// the two book tiers: a never-searched row is offered before any below-target one, and the sweep
// cursor doesn't move while that is still true.
func TestServer_RefillJobBuffer_BuffersUnlearnedBeforePartiallyLearned(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	testPartiallyLearnedPosition(t, s, 12)
	unlearned := testPosition(t, 13)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{unlearned}))

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, buffered)
	require.Equal(t, []string{encodeJobCandidate(tierUnlearned, unlearned)}, bufferedPositions(t, s))
	require.Equal(t, int64(0), s.redis.Exists(ctx, jobCursorKey).Val())
}

// TestServer_RefillJobBuffer_BuffersUnlearnedByDiscCount covers the tier's ordering surviving the
// whole path: the scan's order has to reach the buffer, which workers pop from head first, so the
// shallowest unlearned positions go out first even when the filter drops entries between the two.
func TestServer_RefillJobBuffer_BuffersUnlearnedByDiscCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position20 := testPosition(t, 20)
	position12s := testDistinctPositions(t, 12, 2)
	position13 := testPosition(t, 13)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{
		position20, position13, position12s[0], position12s[1],
	}))

	// One of the two 12-disc positions is already being worked on, so the filter drops it.
	claimed := position12s[0]
	rest := position12s[1]
	if rest.String() < claimed.String() {
		claimed, rest = rest, claimed
	}
	require.NoError(t, s.redis.Set(ctx, claimKey(claimed.String()), "worker-1", claimTTL).Err())

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, buffered)
	require.Equal(t, []string{
		encodeJobCandidate(tierUnlearned, rest),
		encodeJobCandidate(tierUnlearned, position13),
		encodeJobCandidate(tierUnlearned, position20),
	}, bufferedPositions(t, s))
}

// TestServer_RefillJobBuffer_FindsUnlearnedAddedAfterTheSweepMovedOn is why the unlearned scan has
// no cursor: the partially learned sweep is millions of rows long, so a row promoted into the book
// while it is running would otherwise wait for it to wrap.
func TestServer_RefillJobBuffer_FindsUnlearnedAddedAfterTheSweepMovedOn(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	testPartiallyLearnedPosition(t, s, 12)

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, buffered)
	require.NotEqual(t, db.LearnableCursor{}, decodeJobCursor(s.redis.Get(ctx, jobCursorKey).Val()))

	unlearned := testPosition(t, 13)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{unlearned}))

	buffered, err = s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, buffered)
	require.Equal(t, encodeJobCandidate(tierUnlearned, unlearned), bufferedPositions(t, s)[1])
}

// TestServer_RefillJobBuffer_NoMoveRowsDoNotBlockPartiallyLearnedTier covers the candidate that
// would otherwise head every unlearned scan for good: a row edax can't search stays at level 0
// forever, so it neither gets buffered nor counts as unfinished tier-two work.
func TestServer_RefillJobBuffer_NoMoveRowsDoNotBlockPartiallyLearnedTier(t *testing.T) {
	s, tx := testServerWithTx(t)
	ctx := context.Background()

	noMoves := testNoMovesPosition(t, 13)
	_, err := tx.Exec(ctx,
		`INSERT INTO boards (position, disc_count) VALUES ($1, $2)`,
		noMoves.Position().Bytes(), noMoves.CountDiscs())
	require.NoError(t, err)

	partial := testPartiallyLearnedPosition(t, s, 14)

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, buffered)
	require.Equal(t, []string{encodeJobCandidate(tierPartiallyLearned, partial)}, bufferedPositions(t, s))
}

// TestServer_RefillJobBuffer_ClaimedUnlearnedRowHoldsBackPartiallyLearnedTier covers the other
// unusable candidate: a row another worker holds is unfinished tier-two work, so the refill buffers
// nothing rather than starting the partially learned sweep early.
func TestServer_RefillJobBuffer_ClaimedUnlearnedRowHoldsBackPartiallyLearnedTier(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	claimed := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{claimed}))
	require.NoError(t, s.redis.Set(ctx, claimKey(claimed.String()), "worker-1", claimTTL).Err())

	testPartiallyLearnedPosition(t, s, 14)

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Zero(t, buffered)
	require.Empty(t, bufferedPositions(t, s))
}

// testNoMovesPosition returns a normalized position with discs discs whose mover has no legal move:
// with no opponent discs on the board there is nothing to flip.
func testNoMovesPosition(t *testing.T, discs int) othello.NormalizedPosition {
	t.Helper()

	var player uint64
	for i := range uint(discs) {
		player |= 1 << i
	}

	position, err := othello.NewPosition(player, 0)
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	return position.Normalize()
}

func TestServer_RefillJobBuffer_AdvancesCursorThenWraps(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	positions := []othello.NormalizedPosition{
		testPartiallyLearnedPosition(t, s, 12),
		testPartiallyLearnedPosition(t, s, 13),
		testPartiallyLearnedPosition(t, s, 14),
	}

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, len(positions), buffered)

	encoded, err := s.redis.Get(ctx, jobCursorKey).Result()
	require.NoError(t, err)
	require.NotEqual(t, db.LearnableCursor{}, decodeJobCursor(encoded))

	// The sweep is exhausted, so this one wraps and offers the still-below-target positions again.
	buffered, err = s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Equal(t, len(positions), buffered)
}

func TestServer_RefillJobBuffer_ResetsCursorWhenNothingIsLearnable(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: TargetLevel(12), Score: 0}))
	require.NoError(t, s.redis.Set(ctx, jobCursorKey, encodeJobCursor(db.LearnableCursor{
		DiscCount: 12, Level: 24, Position: position,
	}), 0).Err())

	buffered, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)
	require.Zero(t, buffered)

	require.Equal(t, int64(0), s.redis.Exists(ctx, jobCursorKey).Val())
}

func TestServer_RefillJobBuffer_ReleasesItsOwnLock(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	positions := testDistinctPositions(t, 12, 2)
	require.NoError(t, s.repo.AddPositions(ctx, positions))

	_, err := s.refillJobBuffer(ctx)
	require.NoError(t, err)

	require.Equal(t, int64(0), s.redis.Exists(ctx, jobRefillLockKey).Val())
}

func TestServer_PopJobCandidate_SkipsUndecodableEntries(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	// A corrupted entry and a legacy untagged one are both dropped; only the tagged entry is served.
	require.NoError(t, s.redis.RPush(ctx, jobBufferKey,
		"not-a-position", position.String(), encodeJobCandidate(tierPartiallyLearned, position)).Err())

	popped, tier, found, err := s.popJobCandidate(ctx)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, position, popped)
	require.Equal(t, tierPartiallyLearned, tier)
}

func TestServer_PopJobCandidate_EmptyBuffer(t *testing.T) {
	s := testServer(t)

	_, _, found, err := s.popJobCandidate(context.Background())
	require.NoError(t, err)
	require.False(t, found)
}

func TestDecodeJobCandidate(t *testing.T) {
	position := testPosition(t, 12)

	tests := []struct {
		name    string
		encoded string
		tier    jobTier
		ok      bool
	}{
		{"unlearned", encodeJobCandidate(tierUnlearned, position), tierUnlearned, true},
		{"partially learned", encodeJobCandidate(tierPartiallyLearned, position), tierPartiallyLearned, true},
		{"legacy untagged", position.String(), 0, false},
		{"unknown prefix", "x:" + position.String(), 0, false},
		{"empty", "", 0, false},
		{"tag without position", "u:", 0, false},
		{"position unparseable", "u:not-a-position", 0, false},
		{"position not normalized", "u:" + testNonNormalizedPosition(t).String(), 0, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, tier, ok := decodeJobCandidate(test.encoded)
			require.Equal(t, test.ok, ok)
			if test.ok {
				require.Equal(t, position, decoded)
				require.Equal(t, test.tier, tier)
			}
		})
	}
}

// testNonNormalizedPosition returns a position that is not in canonical form. The start position is
// symmetric enough to be its own normal form, so it can't stand in for one.
func testNonNormalizedPosition(t *testing.T) othello.Position {
	t.Helper()

	for _, child := range othello.NewStartPosition().Children() {
		if !child.IsNormalized() {
			return child
		}
	}

	t.Fatal("no non-normalized position among the start position's children")
	return othello.Position{}
}

// TestHandleSubmitJobResult_PriorityBelowLeafDiscsAddsNoRow is the other end of the savable range:
// everything under book.LeafDiscs is minimax-derived, so a frontend analysis request for such a
// position must not leave a row behind however the worker answers it.
func TestHandleSubmitJobResult_PriorityBelowLeafDiscsAddsNoRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, book.LeafDiscs-1)

	for _, level := range []int{PriorityLevel, TargetLevel(position.CountDiscs())} {
		require.Equal(t, 200, submitPriorityResult(t, s, position, level, 6).Code)

		_, err := s.repo.GetPosition(ctx, position.Position())
		require.ErrorIs(t, err, db.ErrPositionNotFound, "level %d", level)
	}
}

// TestLookupEvaluation_IgnoresRowsBelowLeafDiscs covers the frontend side of the same rule: a stray
// sub-leaf row left over from before the guard is not served as a book entry, so the minimax cache
// stays the only source down there.
func TestLookupEvaluation_IgnoresRowsBelowLeafDiscs(t *testing.T) {
	s, tx := testServerWithTx(t)
	ctx := context.Background()

	position := testPosition(t, book.LeafDiscs-1)
	_, err := tx.Exec(ctx,
		`INSERT INTO boards (position, disc_count, level, score) VALUES ($1, $2, $3, 7)`,
		position.Position().Bytes(), position.CountDiscs(), TargetLevel(position.CountDiscs()))
	require.NoError(t, err)

	_, ok, err := s.lookupEvaluation(ctx, position.Position())
	require.NoError(t, err)
	require.False(t, ok)
}
