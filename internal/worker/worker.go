package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

const (
	defaultHeartbeatInterval = time.Minute
	defaultNoJobSleep        = 10 * time.Second
	defaultErrorSleep        = 10 * time.Second
)

// apiClient is the subset of Client's behavior Worker depends on, so tests
// can inject a fake instead of talking to a real server.
type apiClient interface {
	GetJob(ctx context.Context) (Job, bool, error)
	SubmitJobResult(ctx context.Context, board string, level int, eval edax.Evaluation) error
	Heartbeat(ctx context.Context) error
}

// evaluator is the subset of *edax.Process's behavior Worker depends on, so
// tests can inject a fake instead of a real edax subprocess.
type evaluator interface {
	Evaluate(board othello.Board, level int) (edax.Evaluation, error)
}

// Worker repeatedly claims jobs from the API, evaluates them with edax, and
// submits the results, while sending periodic heartbeats so a long-running
// evaluation doesn't lose its job claim to reaping.
type Worker struct {
	api  apiClient
	edax evaluator

	heartbeatInterval time.Duration
	noJobSleep        time.Duration
	errorSleep        time.Duration
}

// New returns a Worker that claims jobs via api and evaluates them via edax.
func New(api apiClient, edax evaluator) *Worker {
	return &Worker{
		api:               api,
		edax:              edax,
		heartbeatInterval: defaultHeartbeatInterval,
		noJobSleep:        defaultNoJobSleep,
		errorSleep:        defaultErrorSleep,
	}
}

// Run blocks, running the job and heartbeat loops until ctx is canceled.
// Callers that want a blocked edax evaluation to actually stop on shutdown
// must close the underlying edax process (e.g. concurrently with Run, on
// ctx.Done()) — canceling ctx alone can't interrupt it, since edax's
// process I/O doesn't take a context.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		w.runHeartbeat(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runJobs(ctx)
	}()

	wg.Wait()
}

// runHeartbeat sends a heartbeat immediately, then every heartbeatInterval,
// until ctx is canceled. Sending one immediately (rather than waiting for
// the first tick) means the worker shows up on the API's worker listing as
// soon as it starts, not up to a full interval later.
func (w *Worker) runHeartbeat(ctx context.Context) {
	if err := w.api.Heartbeat(ctx); err != nil {
		slog.Error("failed to send heartbeat", "error", err)
	}

	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.api.Heartbeat(ctx); err != nil {
				slog.Error("failed to send heartbeat", "error", err)
			}
		}
	}
}

// runJobs repeatedly claims and processes one job at a time until ctx is
// canceled.
func (w *Worker) runJobs(ctx context.Context) {
	for ctx.Err() == nil {
		w.runOneJob(ctx)
	}
}

// runOneJob claims, evaluates, and submits a single job. If none is
// available, or a step fails, it sleeps before returning so the caller's
// loop doesn't spin.
func (w *Worker) runOneJob(ctx context.Context) {
	job, ok, err := w.api.GetJob(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("failed to get job", "error", err)
		sleep(ctx, w.errorSleep)
		return
	}
	if !ok {
		sleep(ctx, w.noJobSleep)
		return
	}

	board, err := othello.ParseBoard(job.Board)
	if err != nil {
		// The server is the only source of jobs and always sends valid,
		// normalized boards; this would indicate a client/server protocol
		// mismatch, not an expected runtime condition.
		slog.Error("received unparseable board from server", "board", job.Board, "error", err)
		return
	}

	eval, err := w.edax.Evaluate(board, job.Level)
	if err != nil {
		if ctx.Err() != nil {
			// Shutdown closed the edax process mid-evaluation.
			return
		}
		slog.Error("failed to evaluate job", "error", err)
		sleep(ctx, w.errorSleep)
		return
	}

	if err := w.api.SubmitJobResult(ctx, job.Board, job.Level, eval); err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("failed to submit job result", "error", err)
		sleep(ctx, w.errorSleep)
		return
	}
}

// sleep waits for d or until ctx is canceled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
