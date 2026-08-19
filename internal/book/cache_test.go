package book

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testRepository returns a Repository backed by a transaction rolled back when the test ends;
// skips the test if FLIPPY_POSTGRES_URL isn't set.
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

var testPosition = othellotest.Position

func TestCache_Get_MissesBeforeRebuild(t *testing.T) {
	repo := testRepository(t)
	c := NewCache(repo)

	_, ok := c.Get(othello.NewStartPosition())
	require.False(t, ok)
}

func TestCache_Rebuild_EmptyDB(t *testing.T) {
	repo := testRepository(t)
	c := NewCache(repo)

	require.NoError(t, c.Rebuild(context.Background()))
	require.Equal(t, 0, c.Len())
}

func TestCache_Rebuild_BackfillsFromLearnedLeaves(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	// A position one ply short of LeafDiscs: all of its children are LeafDiscs-disc positions, the
	// smallest case fully learnable without seeding the entire book.
	position11 := testPosition(t, LeafDiscs-1)
	children := position11.Position().Children()
	require.NotEmpty(t, children)

	normalizedChildren := make([]othello.NormalizedPosition, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, repo.AddPositions(ctx, normalizedChildren))

	childScores := make(map[othello.Position]int, len(normalizedChildren))
	for i, child := range normalizedChildren {
		score := (i % 129) - 64
		require.NoError(t, repo.SaveEvaluation(ctx, child, db.Evaluation{
			Level: 20, Score: score,
		}))
		childScores[child.Position()] = score
	}

	c := NewCache(repo)
	require.NoError(t, c.Rebuild(ctx))
	require.Positive(t, c.Len())

	want := noValue
	for _, score := range childScores {
		if v := -score; v > want {
			want = v
		}
	}

	got, ok := c.Get(position11.Position())
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestCache_Rebuild_IncompleteLeavesLeaveBoardUncovered(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position11 := testPosition(t, LeafDiscs-1)
	children := position11.Position().Children()
	require.Greater(t, len(children), 1)

	normalizedChildren := make([]othello.NormalizedPosition, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, repo.AddPositions(ctx, normalizedChildren))

	// Learn only the first child: coverage of position11 is incomplete.
	require.NoError(t, repo.SaveEvaluation(ctx, normalizedChildren[0], db.Evaluation{
		Level: 20, Score: 1,
	}))

	c := NewCache(repo)
	require.NoError(t, c.Rebuild(ctx))

	_, ok := c.Get(position11.Position())
	require.False(t, ok)
}

func TestCache_Rebuild_ReflectsNewlyLearnedLeaves(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position11 := testPosition(t, LeafDiscs-1)
	children := position11.Position().Children()
	require.Greater(t, len(children), 1)

	normalizedChildren := make([]othello.NormalizedPosition, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, repo.AddPositions(ctx, normalizedChildren))

	c := NewCache(repo)

	// Learn only the first child, rebuild: incomplete, position11 is a miss.
	require.NoError(t, repo.SaveEvaluation(ctx, normalizedChildren[0], db.Evaluation{
		Level: 20, Score: 1,
	}))
	require.NoError(t, c.Rebuild(ctx))
	_, ok := c.Get(position11.Position())
	require.False(t, ok)

	// Learn the rest, rebuild again on the same Cache: now a hit.
	for _, child := range normalizedChildren[1:] {
		require.NoError(t, repo.SaveEvaluation(ctx, child, db.Evaluation{
			Level: 20, Score: 1,
		}))
	}
	require.NoError(t, c.Rebuild(ctx))
	_, ok = c.Get(position11.Position())
	require.True(t, ok)
}
