package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

func TestServer_TryClaim_SecondClaimFails(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = s.tryClaim(ctx, "board-a", "worker-2")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestServer_TryClaim_DistinctBoardsBothSucceed(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = s.tryClaim(ctx, "board-b", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestServer_ReleaseClaim_AllowsReclaim(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.releaseClaim(ctx, "board-a", "worker-1"))

	ok, err = s.tryClaim(ctx, "board-a", "worker-2")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestServer_ReleaseClaim_NoActiveClaimIsNoop(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.releaseClaim(context.Background(), "board-a", "worker-1"))
}

// TestServer_ReleaseClaim_DoesNotRevokeAnotherWorkersClaim covers the race
// where worker-1's claim TTL expired and worker-2 re-claimed the same board:
// worker-1 finishing late and releasing must not delete worker-2's claim.
func TestServer_ReleaseClaim_DoesNotRevokeAnotherWorkersClaim(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// worker-1 claims, then its claim is taken over by worker-2 (simulating the
	// original TTL having expired and worker-2 winning the re-claim).
	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.redis.Set(ctx, claimKey("board-a"), "worker-2", claimTTL).Err())

	// worker-1's late release must leave worker-2's claim intact.
	require.NoError(t, s.releaseClaim(ctx, "board-a", "worker-1"))

	owner, err := s.redis.Get(ctx, claimKey("board-a")).Result()
	require.NoError(t, err)
	require.Equal(t, "worker-2", owner)
}

func TestServer_RebuildBookStats_ProducesExpectedFields(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// One 12-disc board learned at level 20 (a 20@73% search), one unlearned (depth 0, confidence 0).
	boards := testDistinctBoards(t, 12, 2)
	require.NoError(t, s.repo.AddBoards(ctx, boards))
	require.NoError(t, s.repo.SaveEvaluation(ctx, boards[0], db.Evaluation{Level: 20, Score: 2}))

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
		{DiscCount: 12, Depth: 0, Confidence: 0, Count: 7},
		{DiscCount: 12, Depth: 20, Confidence: 73, Count: 2},
		{DiscCount: 13, Depth: 20, Confidence: 73, Count: 3},
	}, entries)
}

func TestServer_JobFloor_FallsBackWhenHashMissing(t *testing.T) {
	s := testServer(t)
	require.Equal(t, book.LeafDiscs, s.jobFloor(context.Background()))
}

func TestServer_JobFloor_PicksLowestLearnableDiscCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// 12-disc board fully learned, 13-disc board unlearned: the floor advances to 13.
	board12 := testBoard(t, 12)
	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12, board13}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board12, db.Evaluation{Level: TargetLevel(12), Score: 0}))
	require.NoError(t, s.rebuildBookStats(ctx))

	require.Equal(t, 13, s.jobFloor(ctx))
}

func TestServer_JobFloor_BelowTargetSearchCountsAsLearnable(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Learned, but below the target level: still learnable, so the floor stays at 12.
	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board12, db.Evaluation{Level: 20, Score: 0}))
	require.NoError(t, s.rebuildBookStats(ctx))

	require.Equal(t, 12, s.jobFloor(ctx))
}

func TestServer_JobFloor_IgnoresDiscCountsOutsideSavableRange(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// An unlearned board above MaxSavableDiscs must not drag the floor up there.
	board35 := testBoard(t, 35)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board35}))
	require.NoError(t, s.rebuildBookStats(ctx))

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

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.redis.Expire(ctx, claimKey("board-a"), time.Second).Err())

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", "board-a"))

	ttl, err := s.redis.TTL(ctx, claimKey("board-a")).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Second)
}

// TestServer_Heartbeat_DoesNotRefreshClaimTakenOverByAnotherWorker covers a
// board whose claim expired and was re-claimed by another worker: the first
// worker's heartbeat must not extend the new owner's claim.
func TestServer_Heartbeat_DoesNotRefreshClaimTakenOverByAnotherWorker(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	// worker-2 takes over the board (as if worker-1's TTL had lapsed).
	require.NoError(t, s.redis.Set(ctx, claimKey("board-a"), "worker-2", time.Second).Err())

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1", "board-a"))

	// worker-2's short TTL must be left untouched.
	ttl, err := s.redis.TTL(ctx, claimKey("board-a")).Result()
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

func TestServer_Heartbeat_UnclaimedBoardIsNotAnError(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "worker-1", "host-1", "commit-1", "board-a"))
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
	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
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
