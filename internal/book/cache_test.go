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

// testBoard returns a NormalizedBoard reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testBoard = othellotest.Board

func TestCache_Get_MissesBeforeRebuild(t *testing.T) {
	repo := testRepository(t)
	c := NewCache(repo)

	_, ok := c.Get(othello.NewBoardStart())
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

	// A real board one ply short of LeafDiscs: all of its children are
	// LeafDiscs-disc boards, the smallest case that can be fully learned
	// without seeding the entire book.
	board11 := testBoard(t, LeafDiscs-1)
	children := board11.Board().Children()
	require.NotEmpty(t, children)

	normalizedChildren := make([]othello.NormalizedBoard, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, repo.AddBoards(ctx, normalizedChildren))

	childScores := make(map[othello.Board]int, len(normalizedChildren))
	for i, child := range normalizedChildren {
		score := (i % 129) - 64
		require.NoError(t, repo.SaveEvaluation(ctx, child, db.Evaluation{
			Level: 20, Score: score,
		}))
		childScores[child.Board()] = score
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

	got, ok := c.Get(board11.Board())
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestCache_Rebuild_IncompleteLeavesLeaveBoardUncovered(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board11 := testBoard(t, LeafDiscs-1)
	children := board11.Board().Children()
	require.Greater(t, len(children), 1)

	normalizedChildren := make([]othello.NormalizedBoard, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, repo.AddBoards(ctx, normalizedChildren))

	// Learn only the first child: coverage of board11 is incomplete.
	require.NoError(t, repo.SaveEvaluation(ctx, normalizedChildren[0], db.Evaluation{
		Level: 20, Score: 1,
	}))

	c := NewCache(repo)
	require.NoError(t, c.Rebuild(ctx))

	_, ok := c.Get(board11.Board())
	require.False(t, ok)
}

func TestCache_Rebuild_ReflectsNewlyLearnedLeaves(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	board11 := testBoard(t, LeafDiscs-1)
	children := board11.Board().Children()
	require.Greater(t, len(children), 1)

	normalizedChildren := make([]othello.NormalizedBoard, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, repo.AddBoards(ctx, normalizedChildren))

	c := NewCache(repo)

	// Learn only the first child, rebuild: incomplete, board11 is a miss.
	require.NoError(t, repo.SaveEvaluation(ctx, normalizedChildren[0], db.Evaluation{
		Level: 20, Score: 1,
	}))
	require.NoError(t, c.Rebuild(ctx))
	_, ok := c.Get(board11.Board())
	require.False(t, ok)

	// Learn the rest, rebuild again on the same Cache: now a hit. This is
	// exactly the "recompute whenever a 12-disc evaluation is saved" case.
	for _, child := range normalizedChildren[1:] {
		require.NoError(t, repo.SaveEvaluation(ctx, child, db.Evaluation{
			Level: 20, Score: 1,
		}))
	}
	require.NoError(t, c.Rebuild(ctx))
	_, ok = c.Get(board11.Board())
	require.True(t, ok)
}
