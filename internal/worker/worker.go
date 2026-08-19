package worker

import (
	"context"
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

	// defaultStatsInterval is how often throughput (boards/sec, sec/board since start) is logged.
	defaultStatsInterval = 10 * time.Second

	// releaseTimeout bounds a best-effort claim release on shutdown, run against a fresh context since
	// Run's ctx is already canceled by the time it happens.
	releaseTimeout = 5 * time.Second
)

// apiClient is the subset of Client's behavior Worker depends on, so tests can inject a fake.
type apiClient interface {
	GetJob(ctx context.Context) (Job, bool, error)
	SubmitJobResult(ctx context.Context, board string, level int, eval edax.Evaluation) error
	ReleaseJob(ctx context.Context, board string) error
	Heartbeat(ctx context.Context, board string) error
}

// evaluator is the subset of *edax.Process's behavior Worker depends on, so tests can inject a fake.
type evaluator interface {
	Evaluate(board othello.Board, level int) (edax.Evaluation, error)
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

	// claimed is the board this worker currently holds a claim on ("" if none), so the heartbeat can
	// refresh it and shutdown can release it.
	mu      sync.Mutex
	claimed string

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
		statsInterval:     defaultStatsInterval,
	}
}

// Run blocks running the job, heartbeat, and stats loops until ctx is canceled; callers must close
// the edax process separately to interrupt a blocked evaluation, since ctx cancellation alone can't.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		w.runHeartbeat(ctx)
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

	// Release the claim still held after a shutdown mid-job, so another worker doesn't have to wait
	// out claimTTL.
	if board := w.claimedBoard(); board != "" {
		w.releaseJob(board)
		w.setClaimedBoard("")
	}
}

// setClaimedBoard records the board this worker currently holds a claim on ("" for none).
func (w *Worker) setClaimedBoard(board string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.claimed = board
}

// claimedBoard returns the board this worker currently holds a claim on, or "".
func (w *Worker) claimedBoard() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.claimed
}

// releaseJob best-effort releases this worker's claim on board.
func (w *Worker) releaseJob(board string) {
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()
	if err := w.api.ReleaseJob(ctx, board); err != nil {
		log.Printf("failed to release claim on %s: %v", board, err)
	}
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

	log.Printf("%d boards done, %.2f boards/sec, %.2f sec/board", count, boardsPerSec, secPerBoard)
}

// runHeartbeat sends a heartbeat immediately, then every heartbeatInterval, until ctx is canceled.
// Each heartbeat reports the currently claimed board so the server can refresh its claim TTL.
func (w *Worker) runHeartbeat(ctx context.Context) {
	if err := w.api.Heartbeat(ctx, w.claimedBoard()); err != nil {
		log.Printf("failed to send heartbeat: %v", err)
	}

	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.api.Heartbeat(ctx, w.claimedBoard()); err != nil {
				log.Printf("failed to send heartbeat: %v", err)
			}
		}
	}
}

// runJobs claims and processes one job at a time until ctx is canceled, sleeping between attempts
// when no job is available or claiming fails.
func (w *Worker) runJobs(ctx context.Context) {
	for ctx.Err() == nil {
		job, ok, err := w.api.GetJob(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("failed to get job: %v", err)
			sleep(ctx, w.errorSleep)
			continue
		}
		if !ok {
			sleep(ctx, w.noJobSleep)
			continue
		}

		w.setClaimedBoard(job.Board)
		if stillClaimed := w.processJob(ctx, job); !stillClaimed {
			w.setClaimedBoard("")
		}
	}
}

// processJob evaluates and submits job, sleeping before returning on failure. It reports whether the
// worker still holds the claim on job's board, which is only the case after a shutdown mid-job; the
// caller releases it then.
func (w *Worker) processJob(ctx context.Context, job Job) (stillClaimed bool) {
	board, err := othello.ParseBoard(job.Board)
	if err != nil {
		// The server always sends valid boards; this indicates a protocol mismatch, not a runtime fluke.
		log.Printf("received unparseable board %q from server: %v", job.Board, err)
		return false
	}

	eval, err := w.edax.Evaluate(board, job.Level)
	if err != nil {
		if ctx.Err() != nil {
			// Shutdown closed the edax process mid-evaluation; the claim is still held and Run
			// releases it.
			return true
		}
		log.Printf("failed to evaluate job: %v", err)
		sleep(ctx, w.errorSleep)
		return false
	}

	if err := w.api.SubmitJobResult(ctx, job.Board, job.Level, eval); err != nil {
		if ctx.Err() != nil {
			return true
		}
		log.Printf("failed to submit job result: %v", err)
		sleep(ctx, w.errorSleep)
		return false
	}

	w.jobsCompleted.Add(1)
	return false
}

// sleep waits for d or until ctx is canceled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
