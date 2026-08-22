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

// testServer returns a Server backed by a rolled-back Postgres transaction and a flushed redis
// database, skipping the test if FLIPPY_POSTGRES_URL or FLIPPY_REDIS_URL isn't set.
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
	return NewServer(repo, redisClient, book.NewCache(repo), testWorkerToken)
}

// testWorkerToken is the worker token every testServer is configured with; doRequest sends it on
// every request.
const testWorkerToken = "test-worker-token"

// testPosition returns a NormalizedPosition reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testPosition = othellotest.Position

// testPassRequiredPosition returns a Position where the player to move must pass but the game is not
// over.
func testPassRequiredPosition(t *testing.T) othello.Position {
	t.Helper()

	position := othello.NewStartPosition()
	for range 200 {
		if !position.HasMoves() {
			passed, err := position.DoMove(othello.PassMove)
			require.NoError(t, err)
			if passed.HasMoves() {
				return position
			}
			t.Fatal("hit a game-ending double pass before finding a single forced pass")
		}

		children := position.Children()
		require.NotEmpty(t, children)
		position = children[0]
	}

	t.Fatal("no forced-pass position found within ply bound")
	return othello.Position{}
}

// testDistinctPositions returns n distinct NormalizedPositions with exactly discs
// discs, found via breadth-first search from the starting position.
var testDistinctPositions = othellotest.DistinctPositions
