package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

func TestServer_ClaimJob_PicksLowestDiscCountThenLevel(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board13s := testDistinctBoards(t, 13, 2)

	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13s[0], board13s[1], board12}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board12, job.Board)
	require.Equal(t, TargetLevel(12), job.Level)
}

func TestServer_ClaimJob_SkipsAlreadyClaimedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	boards := testDistinctBoards(t, 12, 2)
	require.NoError(t, s.repo.AddBoards(ctx, boards))

	job1, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	job2, ok, err := s.claimJob(ctx, "worker-2")
	require.NoError(t, err)
	require.True(t, ok)

	require.NotEqual(t, job1.Board, job2.Board)
}

func TestServer_ClaimJob_SkipsFullyLearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{
		Level: TargetLevel(12), Depth: 24, Confidence: 100, Score: 0,
	}))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestServer_ClaimJob_LeafBoardsDoNotStarveDeeperCandidates covers the
// starvation bug ListLearnable's level cutoff exists to prevent: once every
// 12-disc leaf is fully learned, they still sort ahead of any 13-disc board
// (lower disc count wins regardless of level), so a naive candidate batch
// would consist entirely of already-done leaves and never reach the real
// work below them.
func TestServer_ClaimJob_LeafBoardsDoNotStarveDeeperCandidates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	leafBoards := testDistinctBoards(t, 12, jobCandidateBatch+1)
	require.NoError(t, s.repo.AddBoards(ctx, leafBoards))
	for _, board := range leafBoards {
		require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{
			Level: TargetLevel(12), Depth: 24, Confidence: 100, Score: 0,
		}))
	}

	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board13, job.Board)
	require.Equal(t, TargetLevel(13), job.Level)
}

func TestServer_ClaimJob_SkipsOutOfRangeDiscCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board35 := testBoard(t, 35)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board35}))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestServer_ClaimJob_NoBoardsAvailable(t *testing.T) {
	s := testServer(t)

	_, ok, err := s.claimJob(context.Background(), "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestServer_ClaimJob_AdvancesFloorPastFullyLearnedDiscCount covers the job floor cache: once a claim
// lands above the previous floor, that's proof the skipped disc counts had nothing claimable left, so
// the floor should advance to avoid rescanning them on the next call.
func TestServer_ClaimJob_AdvancesFloorPastFullyLearnedDiscCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board12, db.Evaluation{
		Level: TargetLevel(12), Depth: 24, Confidence: 100, Score: 0,
	}))

	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board13, job.Board)

	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	require.NoError(t, err)
	require.Equal(t, 13, floor)
}

func TestServer_ClaimJob_DoesNotAdvanceFloorWhenClaimingAtFloor(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	require.NoError(t, err)
	require.Equal(t, book.LeafDiscs, floor)
}

// TestServer_ClaimJob_CachedFloorSkipsLowerUnlearnedBoards documents the accepted trade-off of caching
// the floor: while the cache is fresh (within jobFloorTTL), boards below it are invisible to claimJob
// even if unlearned. This only matters for boards added below the floor after it was set (see
// internal/loader.ImportGames), and self-heals once the cache entry expires.
func TestServer_ClaimJob_CachedFloorSkipsLowerUnlearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12, board13}))

	require.NoError(t, s.setJobFloor(ctx, 13))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board13, job.Board)
}
