package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

const (
	defaultHeartbeatInterval = time.Minute
	defaultNoJobSleep        = 10 * time.Second
	defaultErrorSleep        = 10 * time.Second

	// defaultJobBatchSize is how many jobs a single GetJobs call tries to claim at once, and the
	// capacity of the local prefetch queue; defaultJobLowWater is the queue level (in jobs remaining)
	// at which a top-up is triggered, so the queue is refilled before it actually runs dry.
	defaultJobBatchSize = 10
	defaultJobLowWater  = 3

	// defaultStatsInterval is how often throughput (boards/sec, sec/board since start) is logged.
	defaultStatsInterval = 10 * time.Second
)

// apiClient is the subset of Client's behavior Worker depends on, so tests can inject a fake.
type apiClient interface {
	GetJobs(ctx context.Context, count int) ([]Job, error)
	SubmitJobResult(ctx context.Context, board string, level int, eval edax.Evaluation) error
	Heartbeat(ctx context.Context) error
}

// evaluator is the subset of *edax.Process's behavior Worker depends on, so tests can inject a fake.
type evaluator interface {
	Evaluate(board othello.Board, level int) (edax.Evaluation, error)
}

// Worker repeatedly claims jobs from the API, evaluates them with edax, and submits the results.
//
// Jobs are claimed in batches into a local prefetch queue (jobs) rather than one at a time, so
// evaluation doesn't have to wait on a network round trip between jobs; a background goroutine
// (runRefill) tops the queue back up once it drops to jobLowWater, signaled via refill.
type Worker struct {
	api  apiClient
	edax evaluator

	heartbeatInterval time.Duration
	noJobSleep        time.Duration
	errorSleep        time.Duration
	jobBatchSize      int
	jobLowWater       int
	statsInterval     time.Duration

	jobs   chan Job
	refill chan struct{}

	jobsCompleted atomic.Int64
}

// New returns a Worker that claims jobs via api and evaluates them via edax.
func New(api apiClient, edax evaluator) *Worker {
	return &Worker{
		api:               api,
		edax:              edax,
		heartbeatInterval: defaultHeartbeatInterval,
		noJobSleep:        defaultNoJobSleep,
		errorSleep:        defaultErrorSleep,
		jobBatchSize:      defaultJobBatchSize,
		jobLowWater:       defaultJobLowWater,
		statsInterval:     defaultStatsInterval,
		jobs:              make(chan Job, defaultJobBatchSize),
		refill:            make(chan struct{}, 1),
	}
}

// Run blocks running the job, refill, and heartbeat loops until ctx is canceled; callers must close
// the edax process separately to interrupt a blocked evaluation, since ctx cancellation alone can't.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		w.runHeartbeat(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runRefill(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runJobs(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runStats(ctx)
	}()

	wg.Wait()
}

// runStats logs cumulative throughput (boards/sec and sec/board since start) every statsInterval,
// until ctx is canceled.
func (w *Worker) runStats(ctx context.Context) {
	start := time.Now()

	ticker := time.NewTicker(w.statsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.logStats(start)
		}
	}
}

// logStats logs boards processed and throughput since start, both boards/sec and sec/board formatted
// to 2 decimals.
func (w *Worker) logStats(start time.Time) {
	elapsed := time.Since(start).Seconds()
	count := w.jobsCompleted.Load()

	var boardsPerSec, secPerBoard float64
	if elapsed > 0 {
		boardsPerSec = float64(count) / elapsed
	}
	if count > 0 {
		secPerBoard = elapsed / float64(count)
	}

	slog.Info("throughput",
		"boards", count,
		"boards_per_sec", fmt.Sprintf("%.2f", boardsPerSec),
		"sec_per_board", fmt.Sprintf("%.2f", secPerBoard),
	)
}

// runHeartbeat sends a heartbeat immediately, then every heartbeatInterval, until ctx is canceled.
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

// runRefill keeps the jobs queue topped up to jobBatchSize until ctx is canceled. While the queue is
// full it waits for a low-water signal from dequeueJob (or ctx cancellation) before fetching again;
// once fetching, it retries immediately as long as the server keeps returning a full batch (more work
// is likely queued right away), and backs off otherwise (including on error or an empty result) so it
// doesn't hammer the server when supply is scarce.
func (w *Worker) runRefill(ctx context.Context) {
	for ctx.Err() == nil {
		need := w.jobBatchSize - len(w.jobs)
		if need <= 0 {
			select {
			case <-ctx.Done():
				return
			case <-w.refill:
			}
			continue
		}

		jobs, err := w.api.GetJobs(ctx, need)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("failed to get jobs", "error", err)
			sleep(ctx, w.errorSleep)
			continue
		}

		for _, job := range jobs {
			select {
			case w.jobs <- job:
			case <-ctx.Done():
				return
			}
		}

		if len(jobs) < need {
			sleep(ctx, w.noJobSleep)
		}
	}
}

// runJobs repeatedly dequeues and processes one job at a time until ctx is canceled.
func (w *Worker) runJobs(ctx context.Context) {
	for ctx.Err() == nil {
		w.runOneJob(ctx)
	}
}

// dequeueJob waits for a job from the queue, signaling runRefill once the queue drops to jobLowWater;
// it returns false only once ctx is canceled.
func (w *Worker) dequeueJob(ctx context.Context) (Job, bool) {
	select {
	case job := <-w.jobs:
		if len(w.jobs) <= w.jobLowWater {
			select {
			case w.refill <- struct{}{}:
			default:
			}
		}
		return job, true
	case <-ctx.Done():
		return Job{}, false
	}
}

// runOneJob dequeues and processes a single job.
func (w *Worker) runOneJob(ctx context.Context) {
	job, ok := w.dequeueJob(ctx)
	if !ok {
		return
	}
	w.processJob(ctx, job)
}

// processJob evaluates and submits job, sleeping before returning on failure.
func (w *Worker) processJob(ctx context.Context, job Job) {
	board, err := othello.ParseBoard(job.Board)
	if err != nil {
		// The server always sends valid boards; this indicates a protocol mismatch, not a runtime fluke.
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

	w.jobsCompleted.Add(1)
}

// sleep waits for d or until ctx is canceled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
