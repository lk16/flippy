package loader

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// testRepository returns a Repository backed by a transaction that's rolled
// back when the test ends, isolating it from other tests sharing the pool.
// It skips the test if FLIPPY_POSTGRES_URL isn't set.
func testRepository(t *testing.T) *db.Repository {
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

	return db.NewRepository(tx)
}

func TestSeedBoards_AddsPrecomputedSet(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	require.NoError(t, SeedBoards(ctx, repo))

	sample := othello.PrecomputedBoards12()[0]
	eval, err := repo.GetBoard(ctx, sample.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval)
}

func TestSeedBoards_IdempotentDoesNotOverwriteLearnedEvaluation(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	require.NoError(t, SeedBoards(ctx, repo))

	sample := othello.PrecomputedBoards12()[0]
	want := db.Evaluation{Level: 24, Score: 5}
	require.NoError(t, repo.SaveEvaluation(ctx, sample, want))

	require.NoError(t, SeedBoards(ctx, repo))

	got, err := repo.GetBoard(ctx, sample.Board())
	require.NoError(t, err)
	require.Equal(t, want, got)
}
