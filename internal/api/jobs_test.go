package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

func TestServer_ClaimJobs_PicksLowestDiscCountThenLevel(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board13s := testDistinctBoards(t, 13, 2)

	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13s[0], board13s[1], board12}))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, board12, jobs[0].Board)
	require.Equal(t, TargetLevel(12), jobs[0].Level)
}

func TestServer_ClaimJobs_ReturnsUpToCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	boards := testDistinctBoards(t, 12, 5)
	require.NoError(t, s.repo.AddBoards(ctx, boards))

	jobs, err := s.claimJobs(ctx, "worker-1", 3)
	require.NoError(t, err)
	require.Len(t, jobs, 3)
}

func TestServer_ClaimJobs_ReturnsFewerThanCountWhenNotEnoughCandidates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	boards := testDistinctBoards(t, 12, 2)
	require.NoError(t, s.repo.AddBoards(ctx, boards))

	jobs, err := s.claimJobs(ctx, "worker-1", 5)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
}

func TestServer_ClaimJobs_SkipsAlreadyClaimedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	boards := testDistinctBoards(t, 12, 2)
	require.NoError(t, s.repo.AddBoards(ctx, boards))

	jobs1, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Len(t, jobs1, 1)

	jobs2, err := s.claimJobs(ctx, "worker-2", 1)
	require.NoError(t, err)
	require.Len(t, jobs2, 1)

	require.NotEqual(t, jobs1[0].Board, jobs2[0].Board)
}

func TestServer_ClaimJobs_SkipsFullyLearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Empty(t, jobs)
}

// TestServer_ClaimJobs_LeafBoardsDoNotStarveDeeperCandidates covers the
// starvation bug ListLearnable's level cutoff exists to prevent: once every
// 12-disc leaf is fully learned, they still sort ahead of any 13-disc board
// (lower disc count wins regardless of level), so a naive candidate batch
// would consist entirely of already-done leaves and never reach the real
// work below them.
func TestServer_ClaimJobs_LeafBoardsDoNotStarveDeeperCandidates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	leafBoards := testDistinctBoards(t, 12, jobCandidateBatch+1)
	require.NoError(t, s.repo.AddBoards(ctx, leafBoards))
	for _, board := range leafBoards {
		require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{
			Level: TargetLevel(12), Score: 0,
		}))
	}

	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, board13, jobs[0].Board)
	require.Equal(t, TargetLevel(13), jobs[0].Level)
}

func TestServer_ClaimJobs_SkipsOutOfRangeDiscCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board35 := testBoard(t, 35)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board35}))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestServer_ClaimJobs_NoBoardsAvailable(t *testing.T) {
	s := testServer(t)

	jobs, err := s.claimJobs(context.Background(), "worker-1", 1)
	require.NoError(t, err)
	require.Empty(t, jobs)
}

// TestServer_ClaimJobs_AdvancesFloorPastFullyLearnedDiscCount covers the job floor cache: once a claim
// lands above the previous floor, that's proof the skipped disc counts had nothing claimable left, so
// the floor should advance to avoid rescanning them on the next call.
func TestServer_ClaimJobs_AdvancesFloorPastFullyLearnedDiscCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board12, db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))

	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, board13, jobs[0].Board)

	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	require.NoError(t, err)
	require.Equal(t, 13, floor)
}

func TestServer_ClaimJobs_AdvancesFloorToHighestClaimedDiscCountInBatch(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board12, db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))

	board13 := testBoard(t, 13)
	board14 := testBoard(t, 14)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13, board14}))

	jobs, err := s.claimJobs(ctx, "worker-1", 2)
	require.NoError(t, err)
	require.Len(t, jobs, 2)

	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	require.NoError(t, err)
	require.Equal(t, 14, floor)
}

func TestServer_ClaimJobs_DoesNotAdvanceFloorWhenClaimingAtFloor(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	require.NoError(t, err)
	require.Equal(t, book.LeafDiscs, floor)
}

// TestServer_ClaimJobs_CachedFloorSkipsLowerUnlearnedBoards documents the accepted trade-off of caching
// the floor: while the cache is fresh (within jobFloorTTL), boards below it are invisible to claimJobs
// even if unlearned. This only matters for boards added below the floor after it was set (see
// internal/loader.ImportGames), and self-heals once the cache entry expires.
func TestServer_ClaimJobs_CachedFloorSkipsLowerUnlearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12, board13}))

	require.NoError(t, s.setJobFloor(ctx, 13))

	jobs, err := s.claimJobs(ctx, "worker-1", 1)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, board13, jobs[0].Board)
}
