package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
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
func testBoard(t *testing.T, discs int) othello.NormalizedBoard {
	t.Helper()

	board := othello.NewBoardStart()
	for board.CountDiscs() < discs {
		if !board.HasMoves() {
			next, err := board.DoMove(othello.PassMove)
			require.NoError(t, err)
			board = next
			continue
		}

		children := board.Children()
		require.NotEmpty(t, children)
		board = children[0]
	}

	return board.Normalize()
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
