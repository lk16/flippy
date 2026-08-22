package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/api"
	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testClient returns a Client against a real in-process api.Server (rolled-back Postgres
// transaction, flushed redis), so client/server wire mismatches can't hide behind a fake. Skips
// the test if FLIPPY_POSTGRES_URL or FLIPPY_REDIS_URL isn't set.
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
	server := api.NewServer(repo, redisClient, book.NewCache(repo), testWorkerToken)

	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	return NewClient(httpServer.URL, workerID, "test-host", "test-commit", testWorkerToken), repo
}

// testWorkerToken is the worker token the in-process test server and its clients share.
const testWorkerToken = "test-worker-token"

// testPosition returns a NormalizedPosition reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testPosition = othellotest.Position

func TestNewClient_HTTPClientHasTimeout(t *testing.T) {
	client := NewClient("http://localhost", "w1", "test-host", "test-commit", testWorkerToken)

	// Zero would mean the timeout-less http.DefaultClient is back.
	require.Positive(t, client.httpClient.Timeout)
}

// TestClient_StalledServerFailsRatherThanBlocks covers a server that accepts the connection and
// then never answers: the call must fail on its own, since the contexts the worker passes in live
// as long as the worker does and won't cut it short.
func TestClient_StalledServerFailsRatherThanBlocks(t *testing.T) {
	// Released by cleanup: the server doesn't cancel a stalled handler's request context on its
	// own once the client hangs up, so blocking on that would leave Close waiting forever.
	stalled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-stalled
	}))
	t.Cleanup(func() {
		close(stalled)
		server.Close()
	})

	client := NewClient(server.URL, "w1", "test-host", "test-commit", testWorkerToken)
	client.httpClient.Timeout = 50 * time.Millisecond

	_, _, err := client.GetJob(context.Background())
	require.Error(t, err)

	// The POST path uses the same client, and has a request body to send.
	require.Error(t, client.Heartbeat(context.Background(), ""))
}

func TestClient_GetJob_NoJobAvailable(t *testing.T) {
	client, _ := testClient(t, "w1")

	_, ok, err := client.GetJob(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestClient_GetJob_ReturnsJob(t *testing.T) {
	client, repo := testClient(t, "w1")
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	job, ok, err := client.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position.String(), job.Position)
	require.Positive(t, job.Level)
}

func TestClient_GetJob_TwoWorkersGetDistinctJobs(t *testing.T) {
	client1, repo := testClient(t, "w1")
	// Same server/DB as client1, different worker identity: testClient
	// gives each caller its own isolated Postgres transaction, which two
	// separate calls would put on different, mutually-invisible snapshots.
	client2 := NewClient(client1.baseURL, "w2", "test-host", "test-commit", testWorkerToken)
	ctx := context.Background()

	positions := testDistinctClientBoards(t, 12, 2)
	require.NoError(t, repo.AddPositions(ctx, positions))

	job1, ok, err := client1.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	job2, ok, err := client2.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	require.NotEqual(t, job1.Position, job2.Position)
}

func TestClient_SubmitJobResult_Success(t *testing.T) {
	client, repo := testClient(t, "w1")
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	job, ok, err := client.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	eval := edax.Evaluation{Score: 6}
	require.NoError(t, client.SubmitJobResult(ctx, position.String(), job.Level, eval))

	stored, err := repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: job.Level, Score: 6}, stored)
}

func TestClient_SubmitJobResult_BoardNotFound(t *testing.T) {
	client, _ := testClient(t, "w1")

	position := testPosition(t, 12)
	eval := edax.Evaluation{Score: 6}
	err := client.SubmitJobResult(context.Background(), position.String(), api.TargetLevel(12), eval)
	require.Error(t, err)
}

func TestClient_Heartbeat_Success(t *testing.T) {
	client, _ := testClient(t, "w1")
	require.NoError(t, client.Heartbeat(context.Background(), ""))
}

func TestClient_Heartbeat_WithClaimedBoard(t *testing.T) {
	client, repo := testClient(t, "w1")
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	job, ok, err := client.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, client.Heartbeat(ctx, job.Position))
}

func TestClient_ReleaseJob_AllowsAnotherWorkerToClaimIt(t *testing.T) {
	client1, repo := testClient(t, "w1")
	client2 := NewClient(client1.baseURL, "w2", "test-host", "test-commit", testWorkerToken)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	job, ok, err := client1.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, client1.ReleaseJob(ctx, job.Position))

	job2, ok, err := client2.GetJob(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, job.Position, job2.Position)
}

// testDistinctClientBoards returns n distinct NormalizedPositions with exactly
// discs discs, found via breadth-first search from the starting position.
func testDistinctClientBoards(t *testing.T, discs, n int) []othello.NormalizedPosition {
	t.Helper()

	seen := make(map[othello.Position]bool)
	var result []othello.NormalizedPosition

	frontier := []othello.Position{othello.NewStartPosition()}
	for len(frontier) > 0 && len(result) < n {
		var next []othello.Position
		for _, position := range frontier {
			if position.CountDiscs() == discs {
				norm := position.Normalize()
				if key := norm.Position(); !seen[key] {
					seen[key] = true
					result = append(result, norm)
				}
				continue
			}

			if !position.HasMoves() {
				passed, err := position.DoMove(othello.PassMove)
				require.NoError(t, err)
				next = append(next, passed)
				continue
			}

			next = append(next, position.Children()...)
		}
		frontier = next
	}

	require.GreaterOrEqual(t, len(result), n)
	return result[:n]
}
