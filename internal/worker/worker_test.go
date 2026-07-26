package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

// fakeAPIClient is a test double for apiClient. Each method delegates to a
// function field, set per test, so behavior (including which calls happen
// concurrently with a running Worker) stays in the test itself.
type fakeAPIClient struct {
	mu sync.Mutex

	getJob          func(ctx context.Context) (Job, bool, error)
	submitJobResult func(ctx context.Context, board string, level int, eval edax.Evaluation) error
	heartbeat       func(ctx context.Context) error

	submitCalls    []jobResultRequest
	heartbeatCalls int
}

func (f *fakeAPIClient) GetJob(ctx context.Context) (Job, bool, error) {
	return f.getJob(ctx)
}

func (f *fakeAPIClient) SubmitJobResult(ctx context.Context, board string, level int, eval edax.Evaluation) error {
	f.mu.Lock()
	f.submitCalls = append(f.submitCalls, jobResultRequest{
		Board: board, Level: level, Depth: eval.Depth, Confidence: eval.Confidence, Score: eval.Score,
	})
	f.mu.Unlock()
	return f.submitJobResult(ctx, board, level, eval)
}

func (f *fakeAPIClient) Heartbeat(ctx context.Context) error {
	f.mu.Lock()
	f.heartbeatCalls++
	f.mu.Unlock()
	return f.heartbeat(ctx)
}

func (f *fakeAPIClient) submitCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.submitCalls)
}

func (f *fakeAPIClient) heartbeatCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.heartbeatCalls
}

// fakeEvaluator is a test double for evaluator.
type fakeEvaluator struct {
	evaluate func(board othello.Board, level int) (edax.Evaluation, error)
}

func (f *fakeEvaluator) Evaluate(board othello.Board, level int) (edax.Evaluation, error) {
	return f.evaluate(board, level)
}

func testWorker(api apiClient, eval evaluator) *Worker {
	return &Worker{
		api:               api,
		edax:              eval,
		heartbeatInterval: time.Millisecond,
		noJobSleep:        time.Millisecond,
		errorSleep:        time.Millisecond,
	}
}

func TestNew_SetsDefaultIntervals(t *testing.T) {
	w := New(&fakeAPIClient{}, &fakeEvaluator{})

	require.Equal(t, defaultHeartbeatInterval, w.heartbeatInterval)
	require.Equal(t, defaultNoJobSleep, w.noJobSleep)
	require.Equal(t, defaultErrorSleep, w.errorSleep)
}

func TestSleep_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	sleep(ctx, time.Hour)
	require.Less(t, time.Since(start), time.Second)
}

func TestSleep_WaitsFullDuration(t *testing.T) {
	start := time.Now()
	sleep(context.Background(), 20*time.Millisecond)
	require.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond)
}

func TestWorker_RunOneJob_NoJobAvailable(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) { return Job{}, false, nil },
	}
	w := testWorker(api, &fakeEvaluator{})

	w.runOneJob(context.Background())

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_RunOneJob_GetJobError(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) { return Job{}, false, errors.New("boom") },
	}
	w := testWorker(api, &fakeEvaluator{})

	w.runOneJob(context.Background())

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_RunOneJob_EvaluatesAndSubmitsResult(t *testing.T) {
	board := othello.NewBoardStart()

	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: board.String(), Level: 24}, true, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(b othello.Board, level int) (edax.Evaluation, error) {
			require.Equal(t, board, b)
			require.Equal(t, 24, level)
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}
	w := testWorker(api, eval)

	w.runOneJob(context.Background())

	require.Equal(t, []jobResultRequest{
		{Board: board.String(), Level: 24, Depth: 24, Confidence: 100, Score: 6},
	}, api.submitCalls)
}

func TestWorker_RunOneJob_UnparseableBoardIsSkipped(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: "not-a-board", Level: 24}, true, nil
		},
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			t.Fatal("Evaluate must not be called for an unparseable board")
			return edax.Evaluation{}, nil
		},
	}
	w := testWorker(api, eval)

	w.runOneJob(context.Background())

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_RunOneJob_EvaluateError(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: othello.NewBoardStart().String(), Level: 24}, true, nil
		},
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) { return edax.Evaluation{}, errors.New("boom") },
	}
	w := testWorker(api, eval)

	w.runOneJob(context.Background())

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_RunOneJob_EvaluateErrorDuringShutdownReturnsQuietly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: othello.NewBoardStart().String(), Level: 24}, true, nil
		},
	}
	eval := &fakeEvaluator{
		// Simulates the edax process having been killed by a concurrent
		// shutdown while Evaluate was blocked on it.
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			return edax.Evaluation{}, errors.New("process killed")
		},
	}
	w := testWorker(api, eval)

	w.runOneJob(ctx)

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_RunOneJob_SubmitError(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: othello.NewBoardStart().String(), Level: 24}, true, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return errors.New("boom") },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) { return edax.Evaluation{}, nil },
	}
	w := testWorker(api, eval)

	w.runOneJob(context.Background())

	// SubmitJobResult was attempted (and recorded) even though it failed.
	require.Equal(t, 1, api.submitCallCount())
}

func TestWorker_Run_StopsOnContextCancellation(t *testing.T) {
	api := &fakeAPIClient{
		getJob:    func(context.Context) (Job, bool, error) { return Job{}, false, nil },
		heartbeat: func(context.Context) error { return nil },
	}
	w := testWorker(api, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}

func TestWorker_Run_SendsHeartbeats(t *testing.T) {
	api := &fakeAPIClient{
		getJob:    func(context.Context) (Job, bool, error) { return Job{}, false, nil },
		heartbeat: func(context.Context) error { return nil },
	}
	w := testWorker(api, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return api.heartbeatCallCount() >= 3
	}, time.Second, time.Millisecond)
}
