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

func TestServer_Heartbeat_RefreshesClaimTTL(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	ok, err := s.tryClaim(ctx, "board-a", "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, s.redis.Expire(ctx, claimKey("board-a"), time.Second).Err())

	require.NoError(t, s.heartbeat(ctx, "worker-1"))

	ttl, err := s.redis.TTL(ctx, claimKey("board-a")).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Second)
}

func TestServer_Heartbeat_IdleWorkerIsNoop(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "worker-unknown"))
}
