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

	getJobs         func(ctx context.Context, count int) ([]Job, error)
	submitJobResult func(ctx context.Context, board string, level int, eval edax.Evaluation) error
	heartbeat       func(ctx context.Context) error

	getJobsCalls   []int
	submitCalls    []jobResultRequest
	heartbeatCalls int
}

func (f *fakeAPIClient) GetJobs(ctx context.Context, count int) ([]Job, error) {
	f.mu.Lock()
	f.getJobsCalls = append(f.getJobsCalls, count)
	f.mu.Unlock()
	return f.getJobs(ctx, count)
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

func (f *fakeAPIClient) getJobsCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.getJobsCalls)
}

// fakeEvaluator is a test double for evaluator.
type fakeEvaluator struct {
	evaluate func(board othello.Board, level int) (edax.Evaluation, error)
}

func (f *fakeEvaluator) Evaluate(board othello.Board, level int) (edax.Evaluation, error) {
	return f.evaluate(board, level)
}

// testWorker returns a Worker with fast intervals and a small job queue (capacity 3, low water 1),
// suitable for deterministic tests of dequeueJob/runRefill interaction.
func testWorker(api apiClient, eval evaluator) *Worker {
	return &Worker{
		api:               api,
		edax:              eval,
		heartbeatInterval: time.Millisecond,
		noJobSleep:        time.Millisecond,
		errorSleep:        time.Millisecond,
		jobBatchSize:      3,
		jobLowWater:       1,
		jobs:              make(chan Job, 3),
		refill:            make(chan struct{}, 1),
	}
}

func TestNew_SetsDefaults(t *testing.T) {
	w := New(&fakeAPIClient{}, &fakeEvaluator{})

	require.Equal(t, defaultHeartbeatInterval, w.heartbeatInterval)
	require.Equal(t, defaultNoJobSleep, w.noJobSleep)
	require.Equal(t, defaultErrorSleep, w.errorSleep)
	require.Equal(t, defaultJobBatchSize, w.jobBatchSize)
	require.Equal(t, defaultJobLowWater, w.jobLowWater)
	require.Equal(t, defaultJobBatchSize, cap(w.jobs))
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

// --- dequeueJob ---

func TestWorker_DequeueJob_ReturnsQueuedJob(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})
	job := Job{Board: othello.NewBoardStart().String(), Level: 16}
	w.jobs <- job

	got, ok := w.dequeueJob(context.Background())
	require.True(t, ok)
	require.Equal(t, job, got)
}

func TestWorker_DequeueJob_ReturnsFalseOnContextCancellation(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := w.dequeueJob(ctx)
	require.False(t, ok)
}

func TestWorker_DequeueJob_SignalsRefillAtLowWater(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})
	// jobLowWater is 1: queuing exactly 1 and dequeuing it drops the queue to 0, at or below low water.
	w.jobs <- Job{Board: othello.NewBoardStart().String(), Level: 16}

	_, ok := w.dequeueJob(context.Background())
	require.True(t, ok)

	select {
	case <-w.refill:
	default:
		t.Fatal("expected a refill signal after dequeuing to at or below low water")
	}
}

func TestWorker_DequeueJob_DoesNotSignalAboveLowWater(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})
	// jobLowWater is 1: queuing 3 (capacity) and dequeuing one leaves 2 in the queue, above low water.
	for range 3 {
		w.jobs <- Job{Board: othello.NewBoardStart().String(), Level: 16}
	}

	_, ok := w.dequeueJob(context.Background())
	require.True(t, ok)

	select {
	case <-w.refill:
		t.Fatal("did not expect a refill signal while the queue is above low water")
	default:
	}
}

// --- processJob ---

func TestWorker_ProcessJob_EvaluatesAndSubmitsResult(t *testing.T) {
	board := othello.NewBoardStart()

	api := &fakeAPIClient{
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

	w.processJob(context.Background(), Job{Board: board.String(), Level: 24})

	require.Equal(t, []jobResultRequest{
		{Board: board.String(), Level: 24, Depth: 24, Confidence: 100, Score: 6},
	}, api.submitCalls)
}

func TestWorker_ProcessJob_UnparseableBoardIsSkipped(t *testing.T) {
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			t.Fatal("Evaluate must not be called for an unparseable board")
			return edax.Evaluation{}, nil
		},
	}
	w := testWorker(api, eval)

	w.processJob(context.Background(), Job{Board: "not-a-board", Level: 24})

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_EvaluateError(t *testing.T) {
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) { return edax.Evaluation{}, errors.New("boom") },
	}
	w := testWorker(api, eval)

	w.processJob(context.Background(), Job{Board: othello.NewBoardStart().String(), Level: 24})

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_EvaluateErrorDuringShutdownReturnsQuietly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		// Simulates the edax process having been killed by a concurrent
		// shutdown while Evaluate was blocked on it.
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			return edax.Evaluation{}, errors.New("process killed")
		},
	}
	w := testWorker(api, eval)

	w.processJob(ctx, Job{Board: othello.NewBoardStart().String(), Level: 24})

	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_SubmitError(t *testing.T) {
	api := &fakeAPIClient{
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return errors.New("boom") },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) { return edax.Evaluation{}, nil },
	}
	w := testWorker(api, eval)

	w.processJob(context.Background(), Job{Board: othello.NewBoardStart().String(), Level: 24})

	// SubmitJobResult was attempted (and recorded) even though it failed.
	require.Equal(t, 1, api.submitCallCount())
}

// --- runOneJob (dequeueJob + processJob wired together) ---

func TestWorker_RunOneJob_ProcessesQueuedJob(t *testing.T) {
	board := othello.NewBoardStart()

	api := &fakeAPIClient{
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}
	w := testWorker(api, eval)
	w.jobs <- Job{Board: board.String(), Level: 24}

	w.runOneJob(context.Background())

	require.Equal(t, 1, api.submitCallCount())
}

func TestWorker_RunOneJob_ReturnsOnEmptyQueueAndCanceledContext(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w.runOneJob(ctx)
}

// --- runRefill ---

func TestWorker_RunRefill_FillsQueueUpToBatchSize(t *testing.T) {
	board := othello.NewBoardStart().String()

	api := &fakeAPIClient{
		getJobs: func(_ context.Context, count int) ([]Job, error) {
			jobs := make([]Job, count)
			for i := range jobs {
				jobs[i] = Job{Board: board, Level: 16}
			}
			return jobs, nil
		},
	}
	w := testWorker(api, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.runRefill(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		return len(w.jobs) == w.jobBatchSize
	}, time.Second, time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runRefill did not stop after context cancellation")
	}
}

func TestWorker_RunRefill_RequestsExactlyTheRoomLeftInTheQueue(t *testing.T) {
	board := othello.NewBoardStart().String()
	// jobBatchSize is 3; pre-fill the queue with 1, leaving room for 2.
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})
	w.jobs <- Job{Board: board, Level: 16}

	requested := make(chan int, 1)
	api := &fakeAPIClient{
		getJobs: func(_ context.Context, count int) ([]Job, error) {
			requested <- count
			return nil, nil
		},
	}
	w.api = api

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runRefill(ctx)

	select {
	case count := <-requested:
		require.Equal(t, 2, count)
	case <-time.After(time.Second):
		t.Fatal("GetJobs was not called")
	}
}

func TestWorker_RunRefill_WaitsForSignalOncePartiallyFilled(t *testing.T) {
	board := othello.NewBoardStart().String()

	api := &fakeAPIClient{
		getJobs: func(context.Context, int) ([]Job, error) {
			// Fewer than requested: runRefill should back off rather than immediately retry.
			return []Job{{Board: board, Level: 16}}, nil
		},
	}
	w := testWorker(api, &fakeEvaluator{})
	w.noJobSleep = time.Hour // long enough that a spurious immediate retry would be caught below

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runRefill(ctx)

	require.Eventually(t, func() bool {
		return api.getJobsCallCount() >= 1
	}, time.Second, time.Millisecond)

	// Give runRefill a moment to (incorrectly) call GetJobs again if it doesn't back off.
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 1, api.getJobsCallCount())
}

func TestWorker_RunRefill_BacksOffAndRetriesOnError(t *testing.T) {
	var calls int
	api := &fakeAPIClient{
		getJobs: func(context.Context, int) ([]Job, error) {
			calls++
			return nil, errors.New("boom")
		},
	}
	w := testWorker(api, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runRefill(ctx)

	require.Eventually(t, func() bool {
		return api.getJobsCallCount() >= 2
	}, time.Second, time.Millisecond)
}

func TestWorker_RunRefill_WaitsWhenQueueIsFull(t *testing.T) {
	board := othello.NewBoardStart().String()
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})
	for range w.jobBatchSize {
		w.jobs <- Job{Board: board, Level: 16}
	}

	api := &fakeAPIClient{
		getJobs: func(context.Context, int) ([]Job, error) {
			t.Fatal("GetJobs must not be called while the queue is already full")
			return nil, nil
		},
	}
	w.api = api

	ctx, cancel := context.WithCancel(context.Background())
	go w.runRefill(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
}

// --- Run (integration) ---

func TestWorker_Run_StopsOnContextCancellation(t *testing.T) {
	api := &fakeAPIClient{
		getJobs:   func(context.Context, int) ([]Job, error) { return nil, nil },
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
		getJobs:   func(context.Context, int) ([]Job, error) { return nil, nil },
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

func TestWorker_Run_EvaluatesAndSubmitsJobsFromRefill(t *testing.T) {
	board := othello.NewBoardStart()
	var served bool

	api := &fakeAPIClient{
		getJobs: func(context.Context, int) ([]Job, error) {
			if served {
				return nil, nil
			}
			served = true
			return []Job{{Board: board.String(), Level: 24}}, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
		heartbeat:       func(context.Context) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return api.submitCallCount() >= 1
	}, time.Second, time.Millisecond)
}
