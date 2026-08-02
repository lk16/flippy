package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testRepository returns a Repository backed by a transaction that's rolled
// back when the test ends, isolating it from other tests sharing the pool.
// It skips the test if FLIPPY_POSTGRES_URL isn't set.
func testRepository(t *testing.T) *Repository {
	t.Helper()

	url := os.Getenv("FLIPPY_POSTGRES_URL")
	if url == "" {
		t.Skip("FLIPPY_POSTGRES_URL not set; skipping test requiring Postgres")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tx.Rollback(ctx)
	})

	return NewRepository(tx)
}

// testBoard returns a NormalizedBoard reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testBoard = othellotest.Board

// testDistinctBoards returns n distinct NormalizedBoards with exactly discs
// discs, found via breadth-first search from the starting position.
var testDistinctBoards = othellotest.DistinctBoards

func TestEvaluation_IsLearned(t *testing.T) {
	require.False(t, Evaluation{}.IsLearned())
	require.False(t, Evaluation{Depth: 5, Confidence: 50, Score: 2}.IsLearned())
	require.True(t, Evaluation{Level: 16, Depth: 16, Confidence: 100, Score: 3}.IsLearned())
}

func TestRepository_AddBoards_GetBoard(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	eval, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, Evaluation{}, eval)
}

func TestRepository_AddBoards_DoesNotOverwriteExisting(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	saved := Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 4}
	require.NoError(t, repo.SaveEvaluation(ctx, board, saved))

	// Adding the same board again must not clobber the evaluation just saved.
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	eval, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, saved, eval)
}

func TestRepository_AddBoards_Empty(t *testing.T) {
	repo := testRepository(t)
	require.NoError(t, repo.AddBoards(context.Background(), nil))
}

func TestRepository_AddBoardsInserted_CountsOnlyNewRows(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	boards := othello.PrecomputedBoards12()[:5]

	inserted, err := repo.AddBoardsInserted(ctx, boards)
	require.NoError(t, err)
	require.Equal(t, len(boards), inserted)

	// Re-inserting the same boards must count zero new rows (ON CONFLICT DO
	// NOTHING skips them), rather than reporting the batch size again.
	inserted, err = repo.AddBoardsInserted(ctx, boards)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)

	// A batch that overlaps existing rows counts only the genuinely new ones.
	extra := othello.PrecomputedBoards12()[3:8] // 2 overlap, 3 new
	inserted, err = repo.AddBoardsInserted(ctx, extra)
	require.NoError(t, err)
	require.Equal(t, 3, inserted)
}

func TestRepository_AddBoardsInserted_Empty(t *testing.T) {
	repo := testRepository(t)
	inserted, err := repo.AddBoardsInserted(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
}

func TestRepository_AddBoards_Multiple(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	boards := othello.PrecomputedBoards12()[:100]

	require.NoError(t, repo.AddBoards(ctx, boards))

	for _, board := range boards {
		eval, err := repo.GetBoard(ctx, board.Board())
		require.NoError(t, err)
		require.Equal(t, Evaluation{}, eval)
	}
}

func TestRepository_GetBoard_NotFound(t *testing.T) {
	repo := testRepository(t)
	board := testBoard(t, 12)

	_, err := repo.GetBoard(context.Background(), board.Board())
	require.ErrorIs(t, err, ErrBoardNotFound)
}

func TestRepository_GetBoard_NormalizesInput(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board, err := othello.NewBoardStart().DoMove(19)
	require.NoError(t, err)
	require.False(t, board.IsNormalized())

	normalized := board.Normalize()
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{normalized}))

	eval, err := repo.GetBoard(ctx, board)
	require.NoError(t, err)
	require.Equal(t, Evaluation{}, eval)
}

func TestRepository_SaveEvaluation_NotFound(t *testing.T) {
	repo := testRepository(t)
	board := testBoard(t, 12)

	err := repo.SaveEvaluation(context.Background(), board, Evaluation{Level: 20})
	require.ErrorIs(t, err, ErrBoardNotFound)
}

func TestRepository_SaveEvaluation_Updates(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	want := Evaluation{Level: 24, Depth: 24, Confidence: 98, Score: -6}
	require.NoError(t, repo.SaveEvaluation(ctx, board, want))

	got, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestRepository_SaveEvaluation_HigherConfidenceSameLevelUpdates(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	first := Evaluation{Level: 20, Depth: 20, Confidence: 90, Score: 2}
	require.NoError(t, repo.SaveEvaluation(ctx, board, first))

	better := Evaluation{Level: 20, Depth: 20, Confidence: 95, Score: 3}
	require.NoError(t, repo.SaveEvaluation(ctx, board, better))

	got, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, better, got)
}

func TestRepository_SaveEvaluation_NoOpWhenNotBetter(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	first := Evaluation{Level: 20, Depth: 20, Confidence: 90, Score: 2}
	require.NoError(t, repo.SaveEvaluation(ctx, board, first))

	// Same (level, confidence): not an improvement, so this is a no-op, not
	// an error.
	require.NoError(t, repo.SaveEvaluation(ctx, board, first))

	got, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, first, got)
}

func TestRepository_ListLearnable_OrdersByDiscCountThenLevel(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board13s := testDistinctBoards(t, 13, 2)
	board13, board13Other := board13s[0], board13s[1]

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board12, board13, board13Other}))
	require.NoError(t, repo.SaveEvaluation(ctx, board13, Evaluation{Level: 10, Depth: 10, Confidence: 100, Score: 0}))
	require.NoError(t, repo.SaveEvaluation(ctx, board13Other, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 0}))

	results, err := repo.ListLearnable(ctx, 12, 30, 12, 24, 24, 10)
	require.NoError(t, err)
	require.Len(t, results, 3)

	require.Equal(t, board12, results[0].Board)
	require.Equal(t, board13, results[1].Board)
	require.Equal(t, board13Other, results[2].Board)
}

// TestRepository_ListLearnable_LeafLevelDoesNotStarveDeeperCandidates covers
// the scenario ListLearnable's level cutoff exists for: once every board at
// minDiscs has reached leafLevel, a batch ordered by disc count then level
// with no level filter would consist entirely of those (they still sort
// first), even though boards deeper in the tree still need work.
func TestRepository_ListLearnable_LeafLevelDoesNotStarveDeeperCandidates(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	leafBoards := testDistinctBoards(t, 12, 5)
	require.NoError(t, repo.AddBoards(ctx, leafBoards))
	for _, board := range leafBoards {
		require.NoError(t, repo.SaveEvaluation(ctx, board, Evaluation{Level: 24, Depth: 24, Confidence: 100, Score: 0}))
	}

	board13 := testBoard(t, 13)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	// A batch smaller than the number of fully-learned leaves: without the
	// level filter, this would return only leaves and miss board13 entirely.
	results, err := repo.ListLearnable(ctx, 12, 30, 12, 24, 16, 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board13, results[0].Board)
}

// TestRepository_ListLearnable_MinDiscsAboveLeafDiscsKeepsLeafThreshold covers a caller raising minDiscs
// above leafDiscs (e.g. a cached "already fully learned up to here" floor): the leaf-level threshold
// must still apply only to leafDiscs, not to whatever minDiscs happens to be.
func TestRepository_ListLearnable_MinDiscsAboveLeafDiscsKeepsLeafThreshold(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board13 := testBoard(t, 13)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))
	// Above deeperLevel (16) but below leafLevel (24): distinguishes the two thresholds.
	require.NoError(t, repo.SaveEvaluation(ctx, board13, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 0}))

	// minDiscs (13) equals board13's disc count but leafDiscs is still 12, so board13 must be judged
	// against deeperLevel (16), not leafLevel (24); a bug binding the leaf check to minDiscs instead of
	// leafDiscs would misapply the leaf threshold here and wrongly still return board13 as needing level 24.
	results, err := repo.ListLearnable(ctx, 13, 30, 12, 24, 16, 10)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestRepository_ListLearnable_FiltersByDiscCountRange(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board35 := testBoard(t, 35)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board12, board35}))

	results, err := repo.ListLearnable(ctx, 12, 30, 12, 24, 24, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board12, results[0].Board)
}

func TestRepository_ListLearnable_RespectsLimit(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	boards := othello.PrecomputedBoards12()[:5]
	require.NoError(t, repo.AddBoards(ctx, boards))

	results, err := repo.ListLearnable(ctx, 12, 30, 12, 24, 24, 3)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

func TestRepository_ListLearnable_Empty(t *testing.T) {
	repo := testRepository(t)

	results, err := repo.ListLearnable(context.Background(), 12, 30, 12, 24, 24, 10)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestRepository_EvaluatedBoards_OnlyReturnsLearnedBoards(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	boards := othello.PrecomputedBoards12()[:2]
	learned, unlearned := boards[0], boards[1]

	require.NoError(t, repo.AddBoards(ctx, boards))
	require.NoError(t, repo.SaveEvaluation(ctx, learned, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 5}))

	results, err := repo.EvaluatedBoards(ctx, 12)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, learned, results[0].Board)
	require.Equal(t, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 5}, results[0].Evaluation)

	require.NotContains(t, results, BoardEvaluation{Board: unlearned})
}

func TestRepository_EvaluatedBoards_FiltersByDiscCount(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	board13 := testBoard(t, 13)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board12, board13}))
	require.NoError(t, repo.SaveEvaluation(ctx, board12, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 0}))
	require.NoError(t, repo.SaveEvaluation(ctx, board13, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 0}))

	results, err := repo.EvaluatedBoards(ctx, 12)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board12, results[0].Board)
}

func TestRepository_EvaluatedBoards_Empty(t *testing.T) {
	repo := testRepository(t)

	results, err := repo.EvaluatedBoards(context.Background(), 12)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestRepository_Stats(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board12s := testDistinctBoards(t, 12, 2)
	board12a, board12b := board12s[0], board12s[1]
	board13 := testBoard(t, 13)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board12a, board12b, board13}))
	require.NoError(t, repo.SaveEvaluation(ctx, board12a, Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 0}))

	stats, err := repo.Stats(ctx)
	require.NoError(t, err)

	require.Contains(t, stats, LevelStat{DiscCount: 12, Level: 0, Count: 1})
	require.Contains(t, stats, LevelStat{DiscCount: 12, Level: 20, Count: 1})
	require.Contains(t, stats, LevelStat{DiscCount: 13, Level: 0, Count: 1})
}

func TestRepository_SaveEvaluation_NoOpWhenLevelLowerDespiteHigherConfidence(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	first := Evaluation{Level: 20, Depth: 20, Confidence: 90, Score: 2}
	require.NoError(t, repo.SaveEvaluation(ctx, board, first))

	// Lower level beats higher confidence in the lexicographic comparison:
	// this must not update, even though confidence is higher.
	worse := Evaluation{Level: 19, Depth: 19, Confidence: 100, Score: 1}
	require.NoError(t, repo.SaveEvaluation(ctx, board, worse))

	got, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, first, got)
}
