package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
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
	submitJobResult func(ctx context.Context, position string, level int, eval edax.Evaluation) error
	releaseJob      func(ctx context.Context, position string) error
	heartbeat       func(ctx context.Context, position string) error

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

func (f *fakeAPIClient) SubmitJobResult(ctx context.Context, position string, level int, eval edax.Evaluation) error {
	f.mu.Lock()
	f.submitCalls = append(f.submitCalls, jobResultRequest{
		Position: position, Level: level, Depth: eval.Depth, Confidence: eval.Confidence, Score: eval.Score,
	})
	f.mu.Unlock()
	return f.submitJobResult(ctx, position, level, eval)
}

func (f *fakeAPIClient) ReleaseJob(ctx context.Context, position string) error {
	f.mu.Lock()
	f.releaseCalls = append(f.releaseCalls, position)
	f.mu.Unlock()
	return f.releaseJob(ctx, position)
}

func (f *fakeAPIClient) Heartbeat(ctx context.Context, position string) error {
	f.mu.Lock()
	f.heartbeatCalls++
	f.heartbeatBoards = append(f.heartbeatBoards, position)
	f.mu.Unlock()
	return f.heartbeat(ctx, position)
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
	evaluate func(position othello.Position, level int) (edax.Evaluation, error)
}

func (f *fakeEvaluator) Evaluate(position othello.Position, level int) (edax.Evaluation, error) {
	return f.evaluate(position, level)
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
	position := othello.NewStartPosition()

	api := &fakeAPIClient{
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(b othello.Position, level int) (edax.Evaluation, error) {
			require.Equal(t, position, b)
			require.Equal(t, 24, level)
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}
	w := testWorker(api, eval)

	stillClaimed, fatalErr := w.processJob(context.Background(), Job{Position: position.String(), Level: 24})

	require.False(t, stillClaimed)
	require.NoError(t, fatalErr)
	require.Equal(t, []jobResultRequest{
		{Position: position.String(), Level: 24, Depth: 24, Confidence: 100, Score: 6},
	}, api.submitCalls)
}

func TestWorker_ProcessJob_UnparseableBoardIsSkipped(t *testing.T) {
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			t.Fatal("Evaluate must not be called for an unparseable position")
			return edax.Evaluation{}, nil
		},
	}
	w := testWorker(api, eval)

	stillClaimed, fatalErr := w.processJob(context.Background(), Job{Position: "not-a-position", Level: 24})

	require.False(t, stillClaimed)
	require.NoError(t, fatalErr)
	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_EvaluateError(t *testing.T) {
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) { return edax.Evaluation{}, errors.New("boom") },
	}
	w := testWorker(api, eval)

	stillClaimed, fatalErr := w.processJob(context.Background(), Job{Position: othello.NewStartPosition().String(), Level: 24})

	require.False(t, stillClaimed)
	require.NoError(t, fatalErr)
	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_EvaluateErrorDuringShutdownKeepsClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	position := othello.NewStartPosition().String()
	api := &fakeAPIClient{}
	eval := &fakeEvaluator{
		// Simulates the edax process having been killed by a concurrent
		// shutdown while Evaluate was blocked on it.
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			return edax.Evaluation{}, errors.New("process killed")
		},
	}
	w := testWorker(api, eval)

	stillClaimed, fatalErr := w.processJob(ctx, Job{Position: position, Level: 24})

	// The claim is still held; Run releases it after the loops stop.
	require.True(t, stillClaimed)
	require.NoError(t, fatalErr)
	require.Equal(t, 0, api.submitCallCount())
}

func TestWorker_ProcessJob_SubmitError(t *testing.T) {
	api := &fakeAPIClient{
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return errors.New("boom") },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) { return edax.Evaluation{}, nil },
	}
	w := testWorker(api, eval)

	stillClaimed, fatalErr := w.processJob(context.Background(), Job{Position: othello.NewStartPosition().String(), Level: 24})

	require.False(t, stillClaimed)
	require.NoError(t, fatalErr)
	// SubmitJobResult was attempted (and recorded) even though it failed.
	require.Equal(t, 1, api.submitCallCount())
}

// --- runJobs ---

func TestWorker_RunJobs_ClaimsEvaluatesAndSubmits(t *testing.T) {
	position := othello.NewStartPosition()
	var served bool

	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			if served {
				return Job{}, false, nil
			}
			served = true
			return Job{Position: position.String(), Level: 24}, true, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.runJobs(ctx) }()

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
	go func() { _ = w.runJobs(ctx) }()

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
	go func() { _ = w.runJobs(ctx) }()

	require.Eventually(t, func() bool {
		return api.getJobCallCount() >= 2
	}, time.Second, time.Millisecond)
}

func TestWorker_RunJobs_ReturnsOnContextCancellation(t *testing.T) {
	w := testWorker(&fakeAPIClient{}, &fakeEvaluator{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, w.runJobs(ctx)) // must return without calling GetJob (nil function would panic)
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
		_ = w.Run(ctx)
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

	go func() { _ = w.Run(ctx) }()

	require.Eventually(t, func() bool {
		return api.heartbeatCallCount() >= 3
	}, time.Second, time.Millisecond)
}

func TestWorker_Run_HeartbeatReportsClaimedBoard(t *testing.T) {
	position := othello.NewStartPosition().String()

	// evaluate blocks until the heartbeat has reported the claimed position.
	reported := make(chan struct{})
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			<-reported
			return edax.Evaluation{Depth: 24, Confidence: 100, Score: 6}, nil
		},
	}

	var once sync.Once
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Position: position, Level: 16}, true, nil
		},
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
		heartbeat: func(_ context.Context, b string) error {
			if b == position {
				once.Do(func() { close(reported) })
			}
			return nil
		},
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	select {
	case <-reported:
	case <-time.After(time.Second):
		t.Fatal("heartbeat never reported the claimed position")
	}
}

func TestWorker_Run_ReleasesClaimOnShutdownMidEvaluation(t *testing.T) {
	position := othello.NewStartPosition().String()

	// evaluate blocks until the test cancels ctx, simulating a worker mid-search.
	evaluating := make(chan struct{})
	unblock := make(chan struct{})
	var once sync.Once
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			once.Do(func() { close(evaluating) })
			<-unblock
			return edax.Evaluation{}, errors.New("process killed")
		},
	}

	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Position: position, Level: 16}, true, nil
		},
		heartbeat:  func(context.Context, string) error { return nil },
		releaseJob: func(context.Context, string) error { return nil },
	}
	w := testWorker(api, eval)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx)
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

	require.Equal(t, []string{position}, api.releasedBoards())
	require.Empty(t, w.claimedPosition())
}

func TestWorker_Run_ExitsOnEdaxStartFailure(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Position: othello.NewStartPosition().String(), Level: 16}, true, nil
		},
		heartbeat: func(context.Context, string) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			return edax.Evaluation{}, fmt.Errorf("failed to start edax: %w", edax.ErrStartFailed)
		},
	}
	w := testWorker(api, eval)

	err := w.Run(context.Background())
	require.ErrorIs(t, err, edax.ErrStartFailed)
}

func TestWorker_Run_ExitsAfterConsecutiveEvaluateFailures(t *testing.T) {
	api := &fakeAPIClient{
		getJob: func(context.Context) (Job, bool, error) {
			return Job{Position: othello.NewStartPosition().String(), Level: 16}, true, nil
		},
		heartbeat: func(context.Context, string) error { return nil },
	}

	var evalCalls atomic.Int64
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			evalCalls.Add(1)
			return edax.Evaluation{}, errors.New("boom")
		},
	}
	w := testWorker(api, eval)

	err := w.Run(context.Background())
	require.Error(t, err)
	require.NotErrorIs(t, err, edax.ErrStartFailed)
	require.EqualValues(t, maxConsecutiveEvalFailures, evalCalls.Load())
}

func TestWorker_ProcessJob_SuccessResetsFailureStreak(t *testing.T) {
	api := &fakeAPIClient{
		submitJobResult: func(context.Context, string, int, edax.Evaluation) error { return nil },
	}
	eval := &fakeEvaluator{
		evaluate: func(othello.Position, int) (edax.Evaluation, error) {
			return edax.Evaluation{Depth: 16, Confidence: 100, Score: 0}, nil
		},
	}
	w := testWorker(api, eval)
	w.consecutiveEvalFailures = maxConsecutiveEvalFailures - 1

	_, fatalErr := w.processJob(context.Background(), Job{Position: othello.NewStartPosition().String(), Level: 16})

	require.NoError(t, fatalErr)
	require.Zero(t, w.consecutiveEvalFailures)
}

// TestWorker_LogStats covers the throughput line operators read: positions done and the rate they
// came in at, in positions/hour.
func TestWorker_LogStats(t *testing.T) {
	w := New(nil, nil)
	w.jobsCompleted.Store(30)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	w.logStats(time.Now().Add(-2 * time.Hour))

	require.Contains(t, buf.String(), "30 positions done, 15.0 positions/hour")
}

// TestWorker_LogStats_NoElapsedTime covers the divide-by-zero guard: a stats tick that somehow runs
// at the start time reports a rate of zero rather than NaN or an infinity.
func TestWorker_LogStats_NoElapsedTime(t *testing.T) {
	w := New(nil, nil)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	w.logStats(time.Now().Add(time.Hour))

	require.Contains(t, buf.String(), "0 positions done, 0.0 positions/hour")
}
