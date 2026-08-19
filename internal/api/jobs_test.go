package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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
		Level: TargetLevel(12), Score: 0,
	}))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// Covers the starvation bug ListLearnable's level cutoff prevents: fully learned leaves sort
// ahead of deeper unlearned boards and would otherwise fill the whole candidate batch.
func TestServer_ClaimJob_LeafBoardsDoNotStarveDeeperCandidates(t *testing.T) {
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

// TestServer_ClaimJob_StaleFloorSkipsNewlyAddedLowerBoards documents the accepted trade-off of
// deriving the floor from the periodically rebuilt book_stats hash: boards added below the floor
// after the last rebuild are invisible to claimJob until the next rebuild.
func TestServer_ClaimJob_StaleFloorSkipsNewlyAddedLowerBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12s := testDistinctBoards(t, 12, 2)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12s[0]}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board12s[0], db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))
	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	// The rebuilt floor is 13: every 12-disc board known to the hash is fully learned.
	require.NoError(t, s.rebuildBookStats(ctx))

	// A 12-disc board added after the rebuild stays invisible until the next one.
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12s[1]}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board13, job.Board)
}

func TestTargetLevel(t *testing.T) {
	require.Equal(t, 40, TargetLevel(12))
	require.Equal(t, 40, TargetLevel(13))
	require.Equal(t, 36, TargetLevel(14))
	require.Equal(t, 36, TargetLevel(16))
	require.Equal(t, 34, TargetLevel(17))
	require.Equal(t, 34, TargetLevel(20))
	require.Equal(t, 32, TargetLevel(21))
	require.Equal(t, 32, TargetLevel(30))
	require.Equal(t, 32, TargetLevel(64))
}

// TestTargetLevelTiers_MatchTargetLevel guards the contract handleLevelConfig relies on: the tiers
// served to the frontend must reproduce TargetLevel for every disc count, so the frontend's target
// for a board is exactly the one handleAnalyzeRequest clamps its requests to.
func TestTargetLevelTiers_MatchTargetLevel(t *testing.T) {
	tiers := TargetLevelTiers()
	require.NotEmpty(t, tiers)
	require.Equal(t, 64, tiers[len(tiers)-1].MaxDiscs, "last tier must cover a full board")

	for discCount := range 65 {
		var want int
		for _, tier := range tiers {
			if discCount <= tier.MaxDiscs {
				want = tier.Level
				break
			}
		}
		require.Equal(t, want, TargetLevel(discCount), "disc count %d", discCount)
	}
}

// TestIsBookQuality covers the level floor handleSubmitJobResult applies to every submission:
// only a search at least as deep as the board's target level -- or one that already ran the game
// out -- may reach the DB.
func TestIsBookQuality(t *testing.T) {
	tests := []struct {
		name      string
		discCount int
		level     int
		want      bool
	}{
		{"below target", 14, PriorityLevel, false},
		{"one rung below target", 14, TargetLevel(14) - 2, false},
		{"at target", 14, TargetLevel(14), true},
		{"above target", 14, TargetLevel(14) + 2, true},
		{"at target, deepest tier", 30, TargetLevel(30), true},
		// 52 discs is past MaxSavableDiscs, so the disc-count check keeps it out of the DB anyway,
		// but it is the shape the IsFinal clause exists for: a shallow search that is still the
		// game-theoretic result, which no deeper level could improve on.
		{"below target but final", 52, 12, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isBookQuality(tt.discCount, tt.level))
		})
	}
}

// TestTargetLevelTiers_ReturnsACopy makes sure a caller cannot rewrite the table TargetLevel reads.
func TestTargetLevelTiers_ReturnsACopy(t *testing.T) {
	TargetLevelTiers()[0].Level = 1
	require.Equal(t, 40, TargetLevel(4))
}
