package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func TestServer_TryClaim_RecordsClaimedBoardOnWorker(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	board, err := s.redis.HGet(ctx, workerKey("worker-1"), workerFieldClaimedBoard).Result()
	require.NoError(t, err)
	require.Equal(t, "board-a", board)
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

func TestServer_ReleaseClaim_ClearsClaimedBoardOnWorker(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.releaseClaim(ctx, "board-a", "worker-1"))

	board, err := s.redis.HGet(ctx, workerKey("worker-1"), workerFieldClaimedBoard).Result()
	require.NoError(t, err)
	require.Empty(t, board)
}

func TestServer_ReleaseClaim_NoActiveClaimIsNoop(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.releaseClaim(context.Background(), "board-a", "worker-1"))
}

func TestServer_GetJobFloor_DefaultsWhenUnset(t *testing.T) {
	s := testServer(t)

	floor, err := s.getJobFloor(context.Background(), 12)
	require.NoError(t, err)
	require.Equal(t, 12, floor)
}

func TestServer_GetJobFloor_ReturnsCachedValue(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.setJobFloor(ctx, 18))

	floor, err := s.getJobFloor(ctx, 12)
	require.NoError(t, err)
	require.Equal(t, 18, floor)
}

func TestServer_SetJobFloor_ExpiresWithinJobFloorTTL(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.setJobFloor(ctx, 18))

	ttl, err := s.redis.TTL(ctx, jobFloorKey).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Duration(0))
	require.LessOrEqual(t, ttl, jobFloorTTL)
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

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1"))

	ttl, err := s.redis.TTL(ctx, claimKey("board-a")).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Second)
}

func TestServer_Heartbeat_RecordsHostnameAndGitCommit(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1"))

	values, err := s.redis.HGetAll(ctx, workerKey("worker-1")).Result()
	require.NoError(t, err)
	require.Equal(t, "host-1", values[workerFieldHostname])
	require.Equal(t, "commit-1", values[workerFieldGitCommit])
	require.NotEmpty(t, values[workerFieldLastActive])
}

func TestServer_Heartbeat_IdleWorkerIsNotAnError(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "worker-unknown", "host-1", "commit-1"))
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

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1"))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-1"))

	require.NoError(t, s.heartbeat(ctx, "worker-2", "host-2", "commit-2"))
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

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1"))
	require.NoError(t, s.heartbeat(ctx, "worker-2", "host-2", "commit-2"))

	workers, err := s.listWorkers(ctx)
	require.NoError(t, err)
	require.Len(t, workers, 2)
	require.Equal(t, "worker-2", workers[0].ID)
	require.Equal(t, "worker-1", workers[1].ID)
}

func TestServer_ListWorkers_IncludesClaimAndPositionsComputed(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	require.NoError(t, s.heartbeat(ctx, "worker-1", "host-1", "commit-1"))
	require.NoError(t, s.recordJobCompletion(ctx, "worker-1"))

	workers, err := s.listWorkers(ctx)
	require.NoError(t, err)
	require.Len(t, workers, 1)
	require.Equal(t, "worker-1", workers[0].ID)
	require.Equal(t, "host-1", workers[0].Hostname)
	require.Equal(t, "commit-1", workers[0].GitCommit)
	require.Equal(t, 1, workers[0].PositionsComputed)
	require.False(t, workers[0].LastActive.IsZero())
}
