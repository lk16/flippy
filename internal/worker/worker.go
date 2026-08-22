package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
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

	// defaultStatsInterval is how often throughput (positions/sec, sec/position since start) is logged.
	defaultStatsInterval = 10 * time.Second

	// releaseTimeout bounds a best-effort claim release on shutdown, run against a fresh context since
	// Run's ctx is already canceled by the time it happens.
	releaseTimeout = 5 * time.Second

	// maxConsecutiveEvalFailures is how many evaluations in a row may fail before the worker gives
	// up: a persistently failing edax won't heal by retrying, so exit and let the orchestrator
	// restart the pod (with backoff) instead of looping forever.
	maxConsecutiveEvalFailures = 5
)

// apiClient is the subset of Client's behavior Worker depends on, so tests can inject a fake.
type apiClient interface {
	GetJob(ctx context.Context) (Job, bool, error)
	SubmitJobResult(ctx context.Context, position string, level int, eval edax.Evaluation) error
	ReleaseJob(ctx context.Context, position string) error
	Heartbeat(ctx context.Context, position string) error
}

// evaluator is the subset of *edax.Process's behavior Worker depends on, so tests can inject a fake.
type evaluator interface {
	Evaluate(position othello.Position, level int) (edax.Evaluation, error)
}

// Worker repeatedly claims one job at a time from the API, evaluates it with edax, and submits the
// result.
type Worker struct {
	api  apiClient
	edax evaluator

	heartbeatInterval time.Duration
	noJobSleep        time.Duration
	errorSleep        time.Duration
	statsInterval     time.Duration

	// claimed is the position this worker currently holds a claim on ("" if none), so the heartbeat can
	// refresh it and shutdown can release it.
	mu      sync.Mutex
	claimed string

	jobsCompleted atomic.Int64

	// consecutiveEvalFailures counts evaluations failed in a row; reaching
	// maxConsecutiveEvalFailures makes Run return an error.
	consecutiveEvalFailures int
}

// New returns a Worker that claims jobs via api and evaluates them via edax.
func New(api apiClient, edax evaluator) *Worker {
	return &Worker{
		api:               api,
		edax:              edax,
		heartbeatInterval: defaultHeartbeatInterval,
		noJobSleep:        defaultNoJobSleep,
		errorSleep:        defaultErrorSleep,
		statsInterval:     defaultStatsInterval,
	}
}

// Run blocks running the job, heartbeat, and stats loops until ctx is canceled or edax fails
// unrecoverably (non-nil return); callers must close the edax process separately to interrupt a
// blocked evaluation, since ctx cancellation alone can't.
func (w *Worker) Run(ctx context.Context) error {
	// Canceled when the job loop gives up, so the heartbeat and stats loops die with it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)

	var runErr error
	go func() {
		defer wg.Done()
		defer cancel()
		runErr = w.runJobs(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runHeartbeat(ctx)
	}()
	go func() {
		defer wg.Done()
		w.runStats(ctx)
	}()

	wg.Wait()

	// Release the claim still held after a shutdown mid-job, so another worker doesn't have to wait
	// out claimTTL.
	if position := w.claimedPosition(); position != "" {
		w.releaseJob(position)
		w.setClaimedPosition("")
	}

	return runErr
}

// setClaimedPosition records the position this worker currently holds a claim on ("" for none).
func (w *Worker) setClaimedPosition(position string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.claimed = position
}

// claimedPosition returns the position this worker currently holds a claim on, or "".
func (w *Worker) claimedPosition() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.claimed
}

// releaseJob best-effort releases this worker's claim on position.
func (w *Worker) releaseJob(position string) {
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()
	if err := w.api.ReleaseJob(ctx, position); err != nil {
		log.Printf("failed to release claim on %s: %v", position, err)
	}
}

// runStats logs cumulative throughput (positions/sec and sec/position since start) every statsInterval,
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

// logStats logs positions processed and throughput since start, both positions/sec and sec/position formatted
// to 2 decimals.
func (w *Worker) logStats(start time.Time) {
	elapsed := time.Since(start).Seconds()
	count := w.jobsCompleted.Load()

	var positionsPerSec, secPerPosition float64
	if elapsed > 0 {
		positionsPerSec = float64(count) / elapsed
	}
	if count > 0 {
		secPerPosition = elapsed / float64(count)
	}

	log.Printf("%d positions done, %.2f positions/sec, %.2f sec/position", count, positionsPerSec, secPerPosition)
}

// runHeartbeat sends a heartbeat immediately, then every heartbeatInterval, until ctx is canceled.
// Each heartbeat reports the currently claimed position so the server can refresh its claim TTL.
func (w *Worker) runHeartbeat(ctx context.Context) {
	if err := w.api.Heartbeat(ctx, w.claimedPosition()); err != nil {
		log.Printf("failed to send heartbeat: %v", err)
	}

	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.api.Heartbeat(ctx, w.claimedPosition()); err != nil {
				log.Printf("failed to send heartbeat: %v", err)
			}
		}
	}
}

// runJobs claims and processes one job at a time until ctx is canceled, sleeping between attempts
// when no job is available or claiming fails. A non-nil return means edax failed unrecoverably and
// the worker should exit.
func (w *Worker) runJobs(ctx context.Context) error {
	for ctx.Err() == nil {
		job, ok, err := w.api.GetJob(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("failed to get job: %v", err)
			sleep(ctx, w.errorSleep)
			continue
		}
		if !ok {
			sleep(ctx, w.noJobSleep)
			continue
		}

		w.setClaimedPosition(job.Position)
		stillClaimed, fatalErr := w.processJob(ctx, job)
		if !stillClaimed {
			w.setClaimedPosition("")
		}
		if fatalErr != nil {
			return fatalErr
		}
	}
	return nil
}

// processJob evaluates and submits job, sleeping before returning on failure. stillClaimed reports
// whether the worker still holds the claim on job's position, which is only the case after a
// shutdown mid-job; the caller releases it then. A non-nil fatalErr means edax is broken beyond
// what retrying fixes.
func (w *Worker) processJob(ctx context.Context, job Job) (stillClaimed bool, fatalErr error) {
	position, err := othello.ParsePosition(job.Position)
	if err != nil {
		// The server always sends valid positions; this indicates a protocol mismatch, not a runtime fluke.
		log.Printf("received unparseable position %q from server: %v", job.Position, err)
		return false, nil
	}

	eval, err := w.edax.Evaluate(position, job.Level)
	if err != nil {
		if ctx.Err() != nil {
			// Shutdown closed the edax process mid-evaluation; the claim is still held and Run
			// releases it.
			return true, nil
		}
		if errors.Is(err, edax.ErrStartFailed) {
			// A binary that doesn't start won't start next time either.
			return false, err
		}
		w.consecutiveEvalFailures++
		if w.consecutiveEvalFailures >= maxConsecutiveEvalFailures {
			return false, fmt.Errorf("%d consecutive evaluation failures, last: %w",
				w.consecutiveEvalFailures, err)
		}
		log.Printf("failed to evaluate job: %v", err)
		sleep(ctx, w.errorSleep)
		return false, nil
	}
	w.consecutiveEvalFailures = 0

	if err := w.api.SubmitJobResult(ctx, job.Position, job.Level, eval); err != nil {
		if ctx.Err() != nil {
			return true, nil
		}
		log.Printf("failed to submit job result: %v", err)
		sleep(ctx, w.errorSleep)
		return false, nil
	}

	w.jobsCompleted.Add(1)
	return false, nil
}

// sleep waits for d or until ctx is canceled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
