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

// testDistinctBoards returns n distinct NormalizedBoards with exactly discs
// discs, found via breadth-first search from the starting position.
func testDistinctBoards(t *testing.T, discs, n int) []othello.NormalizedBoard {
	t.Helper()

	seen := make(map[othello.Board]bool)
	var result []othello.NormalizedBoard

	frontier := []othello.Board{othello.NewBoardStart()}
	for len(frontier) > 0 && len(result) < n {
		var next []othello.Board
		for _, board := range frontier {
			if board.CountDiscs() == discs {
				norm := board.Normalize()
				if key := norm.Board(); !seen[key] {
					seen[key] = true
					result = append(result, norm)
				}
				continue
			}

			if !board.HasMoves() {
				passed, err := board.DoMove(othello.PassMove)
				require.NoError(t, err)
				next = append(next, passed)
				continue
			}

			next = append(next, board.Children()...)
		}
		frontier = next
	}

	require.GreaterOrEqual(t, len(result), n)
	return result[:n]
}
