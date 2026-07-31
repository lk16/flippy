package api

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testServer returns a Server backed by a Postgres transaction (rolled back
// when the test ends) and a flushed redis database, isolating it from other
// tests. It skips the test if FLIPPY_POSTGRES_URL or FLIPPY_REDIS_URL isn't
// set.
func testServer(t *testing.T) *Server {
	t.Helper()

	pgURL := os.Getenv("FLIPPY_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("FLIPPY_POSTGRES_URL not set; skipping test requiring Postgres")
	}

	redisURL := os.Getenv("FLIPPY_REDIS_URL")
	if redisURL == "" {
		t.Skip("FLIPPY_REDIS_URL not set; skipping test requiring redis")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, pgURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tx.Rollback(ctx)
	})

	redisClient, err := NewRedisClient(ctx, redisURL)
	require.NoError(t, err)
	require.NoError(t, redisClient.FlushDB(ctx).Err())
	t.Cleanup(func() {
		_ = redisClient.FlushDB(ctx)
		_ = redisClient.Close()
	})

	repo := db.NewRepository(tx)
	return NewServer(repo, redisClient, book.NewCache(repo))
}

// testBoard returns a NormalizedBoard reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testBoard = othellotest.Board

// testPassRequiredBoard returns a Board, reached by always playing the
// first available move from the starting position, where the player to
// move has no legal move but the opponent does after the forced pass (as
// opposed to a game-ending double pass).
func testPassRequiredBoard(t *testing.T) othello.Board {
	t.Helper()

	board := othello.NewBoardStart()
	for range 200 {
		if !board.HasMoves() {
			passed, err := board.DoMove(othello.PassMove)
			require.NoError(t, err)
			if passed.HasMoves() {
				return board
			}
			t.Fatal("hit a game-ending double pass before finding a single forced pass")
		}

		children := board.Children()
		require.NotEmpty(t, children)
		board = children[0]
	}

	t.Fatal("no forced-pass position found within ply bound")
	return othello.Board{}
}

// testDistinctBoards returns n distinct NormalizedBoards with exactly discs
// discs, found via breadth-first search from the starting position.
var testDistinctBoards = othellotest.DistinctBoards
