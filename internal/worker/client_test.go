package worker

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/api"
	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testClient returns a Client wired up against a real api.Server (backed by
// a Postgres transaction, rolled back when the test ends, and a flushed
// redis database), served in-process. Exercising the real server, not an
// assumed wire format, is the point: it catches client/server mismatches
// that a hand-rolled fake response couldn't. It skips the test if
// FLIPPY_POSTGRES_URL or FLIPPY_REDIS_URL isn't set.
func testClient(t *testing.T, workerID string) (*Client, *db.Repository) {
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

	redisClient, err := api.NewRedisClient(ctx, redisURL)
	require.NoError(t, err)
	require.NoError(t, redisClient.FlushDB(ctx).Err())
	t.Cleanup(func() {
		_ = redisClient.FlushDB(ctx)
		_ = redisClient.Close()
	})

	repo := db.NewRepository(tx)
	server := api.NewServer(repo, redisClient, book.NewCache(repo))

	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	return NewClient(httpServer.URL, workerID, "test-host", "test-commit"), repo
}

// testBoard returns a NormalizedBoard reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testBoard = othellotest.Board

func TestClient_GetJob_NoJobAvailable(t *testing.T) {
	client, _ := testClient(t, "w1")

	_, ok, err := client.GetJob(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestClient_GetJob_ReturnsJob(t *testing.T) {
	client, repo := testClient(t, "w1")
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	job, ok, err := client.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board.String(), job.Board)
	require.Positive(t, job.Level)
}

func TestClient_GetJob_TwoWorkersGetDistinctJobs(t *testing.T) {
	client1, repo := testClient(t, "w1")
	// Same server/DB as client1, different worker identity: testClient
	// gives each caller its own isolated Postgres transaction, which two
	// separate calls would put on different, mutually-invisible snapshots.
	client2 := NewClient(client1.baseURL, "w2", "test-host", "test-commit")
	ctx := context.Background()

	boards := testDistinctClientBoards(t, 12, 2)
	require.NoError(t, repo.AddBoards(ctx, boards))

	job1, ok, err := client1.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	job2, ok, err := client2.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	require.NotEqual(t, job1.Board, job2.Board)
}

func TestClient_SubmitJobResult_Success(t *testing.T) {
	client, repo := testClient(t, "w1")
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	_, ok, err := client.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	eval := edax.Evaluation{Depth: 24, Confidence: 73, Score: 6}
	require.NoError(t, client.SubmitJobResult(ctx, board.String(), 24, eval))

	stored, err := repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: 24, Score: 6}, stored)
}

func TestClient_SubmitJobResult_BoardNotFound(t *testing.T) {
	client, _ := testClient(t, "w1")

	board := testBoard(t, 12)
	eval := edax.Evaluation{Depth: 24, Confidence: 73, Score: 6}
	err := client.SubmitJobResult(context.Background(), board.String(), 24, eval)
	require.Error(t, err)
}

func TestClient_Heartbeat_Success(t *testing.T) {
	client, _ := testClient(t, "w1")
	require.NoError(t, client.Heartbeat(context.Background(), ""))
}

func TestClient_Heartbeat_WithClaimedBoard(t *testing.T) {
	client, repo := testClient(t, "w1")
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	job, ok, err := client.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, client.Heartbeat(ctx, job.Board))
}

func TestClient_ReleaseJob_AllowsAnotherWorkerToClaimIt(t *testing.T) {
	client1, repo := testClient(t, "w1")
	client2 := NewClient(client1.baseURL, "w2", "test-host", "test-commit")
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	job, ok, err := client1.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, client1.ReleaseJob(ctx, job.Board))

	job2, ok, err := client2.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, job.Board, job2.Board)
}

// testDistinctClientBoards returns n distinct NormalizedBoards with exactly
// discs discs, found via breadth-first search from the starting position.
func testDistinctClientBoards(t *testing.T, discs, n int) []othello.NormalizedBoard {
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
