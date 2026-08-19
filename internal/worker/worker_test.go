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
	releaseJob      func(ctx context.Context, board string) error
	heartbeat       func(ctx context.Context, board string) error

	getJobCalls     int
	submitCalls     []jobResultRequest
	releaseCalls    []string
	heartbeatCalls  int
	heartbeatBoards []string
}

func (f *fakeAPIClient) GetJob(ctx context.Context) (Job, bool, error) {
	f.mu.Lock()
	f.getJobCalls++
	f.mu.Unlock()
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

func (f *fakeAPIClient) ReleaseJob(ctx context.Context, board string) error {
	f.mu.Lock()
	f.releaseCalls = append(f.releaseCalls, board)
	f.mu.Unlock()
	return f.releaseJob(ctx, board)
}

func (f *fakeAPIClient) Heartbeat(ctx context.Context, board string) error {
	f.mu.Lock()
	f.heartbeatCalls++
	f.heartbeatBoards = append(f.heartbeatBoards, board)
	f.mu.Unlock()
	return f.heartbeat(ctx, board)
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

func (f *fakeAPIClient) getJobCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getJobCalls
}

func (f *fakeAPIClient) releasedBoards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.releaseCalls...)
}

// fakeEvaluator is a test double for evaluator.
type fakeEvaluator struct {
	evaluate func(board othello.Board, level int) (edax.Evaluation, error)
}

func (f *fakeEvaluator) Evaluate(board othello.Board, level int) (edax.Evaluation, error) {
	return f.evaluate(board, level)
}

// testWorker returns a Worker with fast intervals for deterministic tests.
func testWorker(api apiClient, eval evaluator) *Worker {
	return &Worker{
		api:               api,
		edax:              eval,
		heartbeatInterval: time.Millisecond,
		noJobSleep:        time.Millisecond,
		errorSleep:        time.Millisecond,
		statsInterval:     time.Hour,
	}
}

func TestNew_SetsDefaults(t *testing.T) {
	w := New(&fakeAPIClient{}, &fakeEvaluator{})

	require.Equal(t, defaultHeartbeatInterval, w.heartbeatInterval)
	require.Equal(t, defaultNoJobSleep, w.noJobSleep)
	require.Equal(t, defaultErrorSleep, w.errorSleep)
	require.Equal(t, defaultStatsInterval, w.statsInterval)
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

	stillClaimed := w.processJob(context.Background(), Job{Board: board.String(), Level: 24})

	require.False(t, stillClaimed)
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

	stillClaimed := w.processJob(context.Background(), Job{Board: "not-a-board", Level: 24})

	require.False(t, stillClaimed)
	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_EvaluateError(t *testing.T) {
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) { return edax.Evaluation{}, errors.New("boom") },
	}
	w := testWorker(api, eval)

	stillClaimed := w.processJob(context.Background(), Job{Board: othello.NewBoardStart().String(), Level: 24})

	require.False(t, stillClaimed)
	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_EvaluateErrorDuringShutdownKeepsClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	board := othello.NewBoardStart().String()
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		// Simulates the edax process having been killed by a concurrent
		// shutdown while Evaluate was blocked on it.
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			return edax.Evaluation{}, errors.New("process killed")
		},
	}
	w := testWorker(api, eval)

	stillClaimed := w.processJob(ctx, Job{Board: board, Level: 24})

	// The claim is still held; Run releases it after the loops stop.
	require.True(t, stillClaimed)
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

	stillClaimed := w.processJob(context.Background(), Job{Board: othello.NewBoardStart().String(), Level: 24})

	require.False(t, stillClaimed)
	// SubmitJobResult was attempted (and recorded) even though it failed.
	require.Equal(t, 1, api.submitCallCount())
}

// --- runJobs ---

func TestWorker_RunJobs_ClaimsEvaluatesAndSubmits(t *testing.T) {
	board := othello.NewBoardStart()
	var served bool

	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			if served {
				return Job{}, false, nil
			}
			served = true
			return Job{Board: board.String(), Level: 24}, true, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runJobs(ctx)

	require.Eventually(t, func() bool {
		return api.submitCallCount() >= 1
	}, time.Second, time.Millisecond)
}

func TestWorker_RunJobs_SleepsWhenNoJobAvailable(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) { return Job{}, false, nil },
	}
	w := testWorker(api, &fakeEvaluator{})
	w.noJobSleep = time.Hour // long enough that a spurious immediate retry would be caught below

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runJobs(ctx)

	require.Eventually(t, func() bool {
		return api.getJobCallCount() >= 1
	}, time.Second, time.Millisecond)

	// Give runJobs a moment to (incorrectly) call GetJob again if it doesn't back off.
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 1, api.getJobCallCount())
}

func TestWorker_RunJobs_BacksOffAndRetriesOnError(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) { return Job{}, false, errors.New("boom") },
	}
	w := testWorker(api, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runJobs(ctx)

	require.Eventually(t, func() bool {
		return api.getJobCallCount() >= 2
	}, time.Second, time.Millisecond)
}

func TestWorker_RunJobs_ReturnsOnContextCancellation(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w.runJobs(ctx) // must return without calling GetJob (nil function would panic)
}

// --- Run (integration) ---

func TestWorker_Run_StopsOnContextCancellation(t *testing.T) {
	api := &fakeAPIClient{
		getJob:    func(context.Context) (Job, bool, error) { return Job{}, false, nil },
		heartbeat: func(context.Context, string) error { return nil },
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
		heartbeat: func(context.Context, string) error { return nil },
	}
	w := testWorker(api, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return api.heartbeatCallCount() >= 3
	}, time.Second, time.Millisecond)
}

func TestWorker_Run_HeartbeatReportsClaimedBoard(t *testing.T) {
	board := othello.NewBoardStart().String()

	// evaluate blocks until the heartbeat has reported the claimed board.
	reported := make(chan struct{})
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			<-reported
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}

	var once sync.Once
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: board, Level: 16}, true, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
		heartbeat: func(_ context.Context, b string) error {
			if b == board {
				once.Do(func() { close(reported) })
			}
			return nil
		},
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	select {
	case <-reported:
	case <-time.After(time.Second):
		t.Fatal("heartbeat never reported the claimed board")
	}
}

func TestWorker_Run_ReleasesClaimOnShutdownMidEvaluation(t *testing.T) {
	board := othello.NewBoardStart().String()

	// evaluate blocks until the test cancels ctx, simulating a worker mid-search.
	evaluating := make(chan struct{})
	unblock := make(chan struct{})
	var once sync.Once
	eval := &fakeEvaluator{
		evaluate: func(othello.Board, int) (edax.Evaluation, error) {
			once.Do(func() { close(evaluating) })
			<-unblock
			return edax.Evaluation{}, errors.New("process killed")
		},
	}

	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Board: board, Level: 16}, true, nil
		},
		heartbeat:  func(context.Context, string) error { return nil },
		releaseJob: func(context.Context, string) error { return nil },
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	<-evaluating
	cancel()
	close(unblock)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}

	require.Equal(t, []string{board}, api.releasedBoards())
	require.Empty(t, w.claimedBoard())
}
