package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

// NewRedisClient opens a redis client for url and verifies it's reachable.
func NewRedisClient(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}

// claimTTL is how long a job claim or worker hash survives without a refresh.
const claimTTL = 5 * time.Minute

// bookStatsKey is the Redis hash of position counts per "<depth>:<confidence>:<discs>" field.
// Saves handled by the server adjust it incrementally (see bookStatsRecordSave); the periodic full
// rebuild only corrects drift, because the underlying GROUP BY is slow at millions of rows.
const bookStatsKey = "book_stats"

// bookStatsRefreshInterval is how often the book_stats hash is fully resynced from the DB,
// correcting drift from loader writes (which bypass the server), Redis restarts, and increments
// racing the swap. In between, the incremental counters keep the hash current.
const bookStatsRefreshInterval = 15 * time.Minute

// bookStatsLockKey elects the one replica that runs a stats resync, so the heavy GROUP BY runs
// once per interval cluster-wide.
const bookStatsLockKey = "book_stats:lock"

// bookStatsLockTTL is slightly shorter than the refresh interval, so the next tick elects a fresh
// runner while a died-mid-hold replica's lock still expires on its own.
const bookStatsLockTTL = bookStatsRefreshInterval - 5*time.Second

// Redis field names within a worker's hash (see workerKey).
const (
	workerFieldHostname          = "hostname"
	workerFieldGitCommit         = "git_commit"
	workerFieldPositionsComputed = "positions_computed"
	workerFieldLastActive        = "last_active"
)

// The shared job candidate buffer: workers pop from jobBufferKey, and jobCursorKey records how far
// through the partially learned rows the current sweep has buffered (unlearned rows are rescanned
// from the start instead). Both are derived state — a refill from an empty cursor rebuilds them.
const (
	jobBufferKey     = "job:buffer"
	jobCursorKey     = "job:cursor"
	jobRefillLockKey = "job:refill:lock"
)

// jobRefillLockTTL bounds how long a replica that died mid-refill blocks other replicas from
// refilling; it only needs to outlast the queries one refill runs.
const jobRefillLockTTL = 30 * time.Second

// How deep the shared buffer is kept and how it is filled. The point of the depth is that a worker's
// claim finds a candidate already waiting: the buffer is topped up in the background, well before it
// runs dry, so no worker ever waits on the DB scan that produces candidates.
const (
	// jobBufferSize is the depth a top-up fills the buffer to. It bounds how long a position can sit
	// buffered before a worker pops it, so it trades scan cost against staleness -- a stale entry is
	// only rejected at claim time, never wrong.
	jobBufferSize = 5000

	// jobBufferLowWater is the depth at or below which the buffer is topped up. Low enough that a
	// top-up covers thousands of claims, high enough that the remaining entries keep workers busy
	// while the refill's queries run.
	jobBufferLowWater = jobBufferSize / 5

	// jobCandidatePage is how many rows one candidate query reads. A refill pages until it has what
	// it was asked for, so this only sets the size of a single query, not how much one refill
	// buffers.
	jobCandidatePage = 1000
)

// claimKey is the redis key holding the worker ID that currently holds position's job, if any.
func claimKey(position string) string {
	return "claim:" + position
}

// workerKey is the redis key of the hash describing a worker's hostname, git commit, stats, and claimed position.
func workerKey(workerID string) string {
	return "worker:" + workerID
}

// tryClaim atomically claims position for workerID, reporting false with no error if already claimed.
func (s *Server) tryClaim(ctx context.Context, position, workerID string) (bool, error) {
	ok, err := s.redis.SetNX(ctx, claimKey(position), workerID, claimTTL).Result()
	if err != nil {
		return false, fmt.Errorf("failed to claim job: %w", err)
	}
	if !ok {
		return false, nil
	}

	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, workerKey(workerID),
		workerFieldLastActive, time.Now().Format(time.RFC3339Nano),
	)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("failed to record worker claim: %w", err)
	}

	return true, nil
}

// releaseClaim deletes the claim on position only if it still belongs to workerID, so a late release
// can't revoke a claim another worker took over. GET-then-DEL is not atomic; the worst case of the
// race is one duplicated evaluation, which is acceptable.
func (s *Server) releaseClaim(ctx context.Context, position, workerID string) error {
	owner, err := s.redis.Get(ctx, claimKey(position)).Result()
	if err == redis.Nil || (err == nil && owner != workerID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to release claim: %w", err)
	}

	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, claimKey(position))
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to release claim: %w", err)
	}

	return nil
}

// jobTier says which book tier buffered a job candidate, so the pop can hold the entry to the tier
// that scanned it rather than re-deriving one from whatever the row holds by then.
type jobTier int

const (
	tierUnlearned jobTier = iota
	tierPartiallyLearned
)

// jobTierPrefixes maps each tier to the prefix its buffer entries carry.
var jobTierPrefixes = map[jobTier]string{
	tierUnlearned:        "u:",
	tierPartiallyLearned: "p:",
}

// encodeJobCandidate renders one buffer entry: the tier's prefix, then the position.
func encodeJobCandidate(tier jobTier, position othello.NormalizedPosition) string {
	return jobTierPrefixes[tier] + position.String()
}

// decodeJobCandidate parses encodeJobCandidate's output; ok is false for anything else, including
// entries from before tiers were tagged -- the buffer is derived state, so dropping those merely
// costs a refill.
func decodeJobCandidate(encoded string) (othello.NormalizedPosition, jobTier, bool) {
	for tier, prefix := range jobTierPrefixes {
		rest, found := strings.CutPrefix(encoded, prefix)
		if !found {
			continue
		}

		position, err := othello.ParsePosition(rest)
		if err != nil {
			return othello.NormalizedPosition{}, 0, false
		}

		normalized, err := othello.NewNormalizedPosition(position)
		if err != nil {
			return othello.NormalizedPosition{}, 0, false
		}

		return normalized, tier, true
	}

	return othello.NormalizedPosition{}, 0, false
}

// popJobCandidate takes the next buffered candidate position and the tier that buffered it,
// reporting found false when the buffer is empty. Popping is what keeps concurrent workers off each
// other's candidates: each entry goes to exactly one of them.
func (s *Server) popJobCandidate(ctx context.Context) (othello.NormalizedPosition, jobTier, bool, error) {
	// An undecodable entry means a corrupted buffer, so drop it and take the next: false has to
	// mean drained, or the caller would refill a buffer that still holds candidates. Bounded by the
	// buffer's depth so one claim can walk past a whole buffer left over from an older encoding.
	for range jobBufferSize {
		encoded, err := s.redis.LPop(ctx, jobBufferKey).Result()
		if err == redis.Nil {
			return othello.NormalizedPosition{}, 0, false, nil
		}
		if err != nil {
			return othello.NormalizedPosition{}, 0, false, fmt.Errorf("failed to pop job candidate: %w", err)
		}

		normalized, tier, ok := decodeJobCandidate(encoded)
		if !ok {
			continue
		}

		return normalized, tier, true, nil
	}

	return othello.NormalizedPosition{}, 0, false, nil
}

// RunJobBufferTopUp keeps the shared job buffer stocked until ctx is canceled, so a worker's claim
// pops a candidate that is already waiting rather than paying for the scan that finds one. Every
// replica polls the buffer's depth -- a single LLEN -- and the refill lock inside refillJobBuffer
// picks the one that does the work.
func (s *Server) RunJobBufferTopUp(ctx context.Context) {
	for {
		delay := jobBufferPollInterval
		if !s.topUpJobBuffer(ctx) {
			delay = jobBufferIdleInterval
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// jobBufferPollInterval is how often a replica checks whether the buffer needs topping up. Well
// under how long jobBufferSize minus jobBufferLowWater candidates take to hand out, so the depth is
// measured many times per refill.
const jobBufferPollInterval = 5 * time.Second

// jobBufferIdleInterval is how long to wait instead when the last refill came up empty: either
// another replica is refilling, or the book has nothing claimable and rescanning it every few
// seconds only burns queries. A worker that asks meanwhile still triggers a refill of its own.
const jobBufferIdleInterval = time.Minute

// topUpJobBuffer refills the buffer to jobBufferSize once it has fallen to the low-water mark,
// reporting whether it is worth asking again soon: false means the last refill buffered nothing.
func (s *Server) topUpJobBuffer(ctx context.Context) bool {
	depth, err := s.redis.LLen(ctx, jobBufferKey).Result()
	if err != nil {
		log.Printf("failed to measure the job buffer: %v", err)
		// A redis hiccup says nothing about the book, so keep the normal interval.
		return true
	}
	if depth > jobBufferLowWater {
		return true
	}

	buffered, err := s.refillJobBuffer(ctx, jobBufferSize-int(depth))
	if err != nil {
		log.Printf("failed to top up the job buffer: %v", err)
		return true
	}

	return buffered > 0
}

// refillJobBuffer buffers candidates until want of them have been added, unlearned positions before
// partially learned ones. It reports how many it buffered; zero means another replica is refilling
// or nothing is claimable right now, and fewer than want means the tier it drew from ran out. The
// partially learned tier only runs once the unlearned tier is exhausted -- unlearned rows already
// claimed or already waiting in the buffer count as unfinished tier-two work, so the sweep waits for
// them rather than starting on below-target rows. Buffered candidates are handed out before the next
// refill looks at the book again, so a position that becomes unlearnable, or an unlearned row added
// meanwhile, waits at most one fill of the buffer.
func (s *Server) refillJobBuffer(ctx context.Context, want int) (int, error) {
	if want <= 0 {
		return 0, nil
	}

	won, err := s.redis.SetNX(ctx, jobRefillLockKey, s.replicaID, jobRefillLockTTL).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to acquire job refill lock: %w", err)
	}
	if !won {
		return 0, nil
	}
	defer s.releaseJobRefillLock(ctx)

	pending, err := s.pendingCandidates(ctx)
	if err != nil {
		return 0, err
	}

	buffered, exhausted, err := s.refillFromUnlearned(ctx, want, pending)
	if err != nil || buffered > 0 {
		return buffered, err
	}
	if !exhausted {
		return 0, nil
	}

	return s.refillFromPartiallyLearned(ctx, s.jobFloor(ctx), want, pending)
}

// pendingCandidates returns the positions still waiting in the buffer, so a refill can skip them:
// the unlearned scan restarts at the top of the book every time and the partially learned sweep
// wraps, so a top-up that runs before the buffer drains would otherwise queue the same positions
// twice. Reading the whole buffer costs one round trip of a few hundred KB, once per refill; an
// entry popped while it is read merely loses a skip.
func (s *Server) pendingCandidates(ctx context.Context) (map[othello.NormalizedPosition]struct{}, error) {
	entries, err := s.redis.LRange(ctx, jobBufferKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read the job buffer: %w", err)
	}

	pending := make(map[othello.NormalizedPosition]struct{}, len(entries))
	for _, entry := range entries {
		position, _, ok := decodeJobCandidate(entry)
		if !ok {
			// Undecodable entries are dropped at pop time; they hold no position back.
			continue
		}
		pending[position] = struct{}{}
	}

	return pending, nil
}

// refillSlackPages is how many pages beyond the ones a refill needs it may read before giving up,
// covering rows it cannot buffer: edax can't search them, another worker holds them, or they are
// already waiting in the buffer. Without it, a book headed by thousands of unusable rows would turn
// one refill into a full index scan.
const refillSlackPages = 10

// refillPageBudget is the most pages one tier's refill reads to buffer want candidates: the pages
// they fit in, plus slack for ones that yield nothing.
func refillPageBudget(want int) int {
	return (want+jobCandidatePage-1)/jobCandidatePage + refillSlackPages
}

// refillFromUnlearned buffers up to want never-searched positions, the second job tier. Every refill
// rescans from the start of the book rather than persisting a cursor, so a row added after an
// earlier refill does not have to wait for the partially-learned sweep to come round; the job floor
// is skipped for the same reason, and costs nothing here because the index this scan runs on holds
// only unlearned rows. It pages until it has want of them -- a page may overshoot, which only leaves
// the buffer a little deeper than asked for -- skipping candidates no worker can take right now,
// since buffering those would hand out unusable entries. Running out of page budget reports the tier
// as busy rather than exhausted: holding the partially learned sweep back is the safe direction, and
// the next refill retries. exhausted is true only when nothing in the tier could ever be buffered
// again as it stands: rows edax can't search (no legal move) stay unlearned forever and so don't
// count, but a claimed or already-buffered row does, since it comes back if the claim expires or the
// worker rejects it.
func (s *Server) refillFromUnlearned(
	ctx context.Context, want int, pending map[othello.NormalizedPosition]struct{},
) (buffered int, exhausted bool, err error) {
	var cursor db.UnlearnedCursor
	spokenFor, budget := 0, refillPageBudget(want)

	for buffered < want {
		if budget == 0 {
			log.Printf("job buffer: %d unlearned candidates in %d pages; treating the tier as busy",
				buffered, refillPageBudget(want))
			return buffered, false, nil
		}
		budget--

		candidates, err := s.repo.ListUnlearned(ctx, db.UnlearnedQuery{
			MinDiscs: book.LeafDiscs,
			MaxDiscs: book.MaxSavableDiscs,
			After:    cursor,
			Limit:    jobCandidatePage,
		})
		if err != nil {
			return buffered, false, fmt.Errorf("failed to list unlearned positions: %w", err)
		}
		if len(candidates) == 0 {
			break
		}

		last := candidates[len(candidates)-1].Position
		cursor = db.UnlearnedCursor{DiscCount: last.CountDiscs(), Position: last}
		lastPage := len(candidates) < jobCandidatePage

		claimable, pageSpokenFor, err := s.claimableCandidates(ctx, candidates, pending)
		if err != nil {
			return buffered, false, err
		}
		spokenFor += pageSpokenFor

		if len(claimable) > 0 {
			if err := s.bufferCandidates(ctx, tierUnlearned, claimable, pending); err != nil {
				return buffered, false, err
			}
			buffered += len(claimable)
		}

		if lastPage {
			break
		}
	}

	if buffered > 0 {
		log.Printf("job buffer: %d unlearned candidates, %d already claimed or buffered", buffered, spokenFor)
	}

	return buffered, buffered == 0 && spokenFor == 0, nil
}

// refillFromPartiallyLearned buffers up to want below-target positions from the sweep cursor, the
// third job tier, wrapping to the start of the ordering once the sweep runs out so positions whose
// claims expired come round again. Positions already waiting in the buffer are skipped, which is
// what keeps a wrap from queueing them a second time.
func (s *Server) refillFromPartiallyLearned(
	ctx context.Context, floor, want int, pending map[othello.NormalizedPosition]struct{},
) (int, error) {
	cursor, err := s.readJobCursor(ctx)
	if err != nil {
		return 0, err
	}

	buffered, wrapped, sawCandidates := 0, false, false
	budget := refillPageBudget(want)
	var next db.LearnableCursor

	for buffered < want && budget > 0 {
		budget--

		candidates, err := s.listPartiallyLearnedFrom(ctx, floor, cursor, jobCandidatePage)
		if err != nil {
			return buffered, err
		}

		if len(candidates) == 0 {
			if wrapped || cursor == (db.LearnableCursor{}) {
				break
			}
			// The sweep ran out; start it over so expired claims come round again.
			cursor, wrapped = db.LearnableCursor{}, true
			continue
		}

		sawCandidates = true
		cursor = candidates[len(candidates)-1].Cursor()
		lastPage := len(candidates) < jobCandidatePage

		// The cursor moves only with the candidates it covers, in one transaction: a page that
		// buffers nothing leaves it where it was, so nothing is skipped unbuffered.
		fresh := freshCandidates(candidates, pending)
		if len(fresh) > 0 {
			next = cursor
			if err := s.bufferCandidates(ctx, tierPartiallyLearned, fresh, pending, func(pipe redis.Pipeliner) {
				pipe.Set(ctx, jobCursorKey, encodeJobCursor(next), 0)
			}); err != nil {
				return buffered, err
			}
			buffered += len(fresh)
		}

		if lastPage {
			break
		}
	}

	if buffered == 0 {
		if sawCandidates {
			// Everything the sweep found is already waiting in the buffer, so there is nothing to
			// add and nothing to conclude about the book; leave the cursor where it is.
			return 0, nil
		}

		// Nothing learnable at all; drop the cursor so the next refill rescans from the start.
		if err := s.redis.Del(ctx, jobCursorKey).Err(); err != nil {
			return 0, fmt.Errorf("failed to reset job cursor: %w", err)
		}
		log.Printf("job buffer: nothing learnable from %d discs up; sweep restarts", floor)
		return 0, nil
	}

	wrapNote := ""
	if wrapped {
		wrapNote = " (wrapped)"
	}
	log.Printf("job buffer: %d partially learned candidates from %d discs up, cursor now %d discs/level %d%s",
		buffered, floor, next.DiscCount, next.Level, wrapNote)

	return buffered, nil
}

// claimableCandidates returns the candidates a worker could take right now -- edax crashes on a
// position with no legal move, another worker already holds the claim on some, and the rest are
// already waiting in the buffer -- along with how many were dropped for being spoken for (claimed or
// buffered), which is what tells a caller the tier still has unfinished work.
func (s *Server) claimableCandidates(
	ctx context.Context, candidates []db.PositionEvaluation, pending map[othello.NormalizedPosition]struct{},
) (claimable []db.PositionEvaluation, spokenFor int, err error) {
	if len(candidates) == 0 {
		return nil, 0, nil
	}

	fresh := freshCandidates(candidates, pending)
	spokenFor = len(candidates) - len(fresh)

	withMoves := make([]db.PositionEvaluation, 0, len(fresh))
	keys := make([]string, 0, len(fresh))
	for _, candidate := range fresh {
		if !candidate.Position.HasMoves() {
			continue
		}
		withMoves = append(withMoves, candidate)
		keys = append(keys, claimKey(candidate.Position.String()))
	}

	if noMoves := len(fresh) - len(withMoves); noMoves > 0 {
		// Rows nothing will ever learn: worth saying out loud, since they stay in the book.
		log.Printf("job buffer: skipped %d candidates with no legal move", noMoves)
	}

	pipe := s.redis.Pipeline()
	exists := make([]*redis.IntCmd, len(keys))
	for i, key := range keys {
		exists[i] = pipe.Exists(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, 0, fmt.Errorf("failed to check candidate claims: %w", err)
	}

	claimable = make([]db.PositionEvaluation, 0, len(withMoves))
	for i, candidate := range withMoves {
		if exists[i].Val() > 0 {
			spokenFor++
			continue
		}
		claimable = append(claimable, candidate)
	}

	return claimable, spokenFor, nil
}

// freshCandidates drops the candidates already waiting in the buffer.
func freshCandidates(
	candidates []db.PositionEvaluation, pending map[othello.NormalizedPosition]struct{},
) []db.PositionEvaluation {
	fresh := make([]db.PositionEvaluation, 0, len(candidates))
	for _, candidate := range candidates {
		if _, buffered := pending[candidate.Position]; buffered {
			continue
		}
		fresh = append(fresh, candidate)
	}
	return fresh
}

// bufferCandidates appends candidates to the shared buffer tagged with the tier that scanned them,
// recording them in pending so later pages of the same refill skip them, and running extra in the
// same transaction so a cursor advance and the candidates it covers land together.
func (s *Server) bufferCandidates(
	ctx context.Context,
	tier jobTier,
	candidates []db.PositionEvaluation,
	pending map[othello.NormalizedPosition]struct{},
	extra ...func(redis.Pipeliner),
) error {
	encoded := make([]any, len(candidates))
	for i, candidate := range candidates {
		encoded[i] = encodeJobCandidate(tier, candidate.Position)
	}

	pipe := s.redis.TxPipeline()
	pipe.RPush(ctx, jobBufferKey, encoded...)
	for _, fn := range extra {
		fn(pipe)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to buffer job candidates: %w", err)
	}

	for _, candidate := range candidates {
		pending[candidate.Position] = struct{}{}
	}

	return nil
}

// listPartiallyLearnedFrom reads up to limit candidates following cursor, from minDiscs up.
func (s *Server) listPartiallyLearnedFrom(
	ctx context.Context, minDiscs int, cursor db.LearnableCursor, limit int,
) ([]db.PositionEvaluation, error) {
	candidates, err := s.repo.ListPartiallyLearned(ctx, db.LearnableQuery{
		MinDiscs:    minDiscs,
		MaxDiscs:    book.MaxSavableDiscs,
		LeafDiscs:   book.LeafDiscs,
		LeafLevel:   TargetLevel(book.LeafDiscs),
		DeeperLevel: TargetLevel(book.LeafDiscs + 1),
		After:       cursor,
		Limit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list candidate positions: %w", err)
	}
	return candidates, nil
}

// releaseJobRefillLock drops the refill lock if this replica still holds it. GET-compare-DEL is not
// atomic; the worst case of the race is two replicas refilling at once, which only duplicates
// buffer entries that tryClaim then rejects.
func (s *Server) releaseJobRefillLock(ctx context.Context) {
	holder, err := s.redis.Get(ctx, jobRefillLockKey).Result()
	if err != nil || holder != s.replicaID {
		return
	}
	if err := s.redis.Del(ctx, jobRefillLockKey).Err(); err != nil {
		log.Printf("failed to release job refill lock: %v", err)
	}
}

// readJobCursor returns how far the current sweep has buffered, or the zero cursor to start over.
func (s *Server) readJobCursor(ctx context.Context) (db.LearnableCursor, error) {
	encoded, err := s.redis.Get(ctx, jobCursorKey).Result()
	if err == redis.Nil {
		return db.LearnableCursor{}, nil
	}
	if err != nil {
		return db.LearnableCursor{}, fmt.Errorf("failed to read job cursor: %w", err)
	}
	return decodeJobCursor(encoded), nil
}

// encodeJobCursor renders cursor for jobCursorKey.
func encodeJobCursor(cursor db.LearnableCursor) string {
	return fmt.Sprintf("%d:%d:%s", cursor.DiscCount, cursor.Level, cursor.Position)
}

// decodeJobCursor parses encodeJobCursor's output, falling back to the zero cursor: the cursor is
// derived state, so restarting the sweep costs a rescan rather than correctness.
func decodeJobCursor(encoded string) db.LearnableCursor {
	fields := strings.SplitN(encoded, ":", 3)
	if len(fields) != 3 {
		return db.LearnableCursor{}
	}

	discCount, err := strconv.Atoi(fields[0])
	if err != nil {
		return db.LearnableCursor{}
	}

	level, err := strconv.Atoi(fields[1])
	if err != nil {
		return db.LearnableCursor{}
	}

	position, err := othello.ParsePosition(fields[2])
	if err != nil {
		return db.LearnableCursor{}
	}

	normalized, err := othello.NewNormalizedPosition(position)
	if err != nil {
		return db.LearnableCursor{}
	}

	return db.LearnableCursor{DiscCount: discCount, Level: level, Position: normalized}
}

// RunBookStatsRefresh attempts a book_stats resync immediately and then every
// bookStatsRefreshInterval, until ctx is canceled; each attempt runs only on the replica that
// wins that interval's lock.
func (s *Server) RunBookStatsRefresh(ctx context.Context) {
	s.refreshBookStatsIfElected(ctx)

	ticker := time.NewTicker(bookStatsRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshBookStatsIfElected(ctx)
		}
	}
}

// refreshBookStatsIfElected rebuilds the book_stats hash only if this replica wins the refresh
// lock; losing the SET means another replica covered this interval. No leader election needed: a
// dead holder's lock expires and any replica picks it up on its next tick.
func (s *Server) refreshBookStatsIfElected(ctx context.Context) {
	won, err := s.redis.SetNX(ctx, bookStatsLockKey, s.replicaID, bookStatsLockTTL).Result()
	if err != nil {
		log.Printf("failed to acquire book stats refresh lock: %v", err)
		return
	}
	if !won {
		return
	}

	if err := s.rebuildBookStats(ctx); err != nil {
		log.Printf("failed to rebuild book stats: %v", err)
	}
}

// rebuildBookStats queries the DB and replaces the book_stats hash, writing into a temp key and
// atomically swapping it in with RENAME so readers never see a partial hash.
func (s *Server) rebuildBookStats(ctx context.Context) error {
	start := time.Now()

	stats, err := s.repo.Stats(ctx)
	if err != nil {
		return fmt.Errorf("failed to query stats: %w", err)
	}

	rows := 0
	for _, stat := range stats {
		rows += stat.Count
	}

	entries := statEntries(stats)
	log.Printf("book stats: resynced %d rows into %d cells in %s", rows, len(entries), time.Since(start).Round(time.Millisecond))
	if len(entries) == 0 {
		// RENAME fails on a missing source key; an absent hash already means "fall back to the DB".
		if err := s.redis.Del(ctx, bookStatsKey).Err(); err != nil {
			return fmt.Errorf("failed to clear book stats: %w", err)
		}
		return nil
	}

	fields := make([]any, 0, 2*len(entries))
	for _, e := range entries {
		fields = append(fields, fmt.Sprintf("%d:%d:%d", e.Depth, e.Confidence, e.DiscCount), e.Count)
	}

	tempKey := bookStatsKey + ":rebuild"
	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, tempKey)
	pipe.HSet(ctx, tempKey, fields...)
	pipe.Rename(ctx, tempKey, bookStatsKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to rebuild book stats: %w", err)
	}

	return nil
}

// flushAndRebuildRedis drops every Redis value and rebuilds the derived ones, for a rollout that
// changed how values are encoded. Only book_stats needs an eager rebuild; everything else -- the
// job buffer, claims, worker hashes, and the ephemeral caches -- repopulates on its own. Dropped
// claims cost at most one duplicated evaluation per active worker.
func (s *Server) flushAndRebuildRedis(ctx context.Context) error {
	if err := s.redis.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to flush redis: %w", err)
	}
	log.Printf("redis: flushed all values on request; rebuilding book stats")
	return s.rebuildBookStats(ctx)
}

// bookStatsField returns the book_stats hash field for discCount-disc positions evaluated at
// level; level 0 (unlearned) reports depth 0, confidence 0, matching statEntries.
func bookStatsField(discCount, level int) string {
	depth, confidence := 0, 0
	if level > 0 {
		depth, confidence = edax.SearchParams(discCount, level)
	}
	return fmt.Sprintf("%d:%d:%d", depth, confidence, discCount)
}

// bookStatsDeltaScript applies HINCRBY deltas ((field, delta) pairs in ARGV) to the book_stats
// hash, but only when it already exists: creating it here would present a nearly-empty hash as
// authoritative until the next full resync.
var bookStatsDeltaScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	return 0
end
for i = 1, #ARGV, 2 do
	redis.call('HINCRBY', KEYS[1], ARGV[i], ARGV[i + 1])
end
return 1`)

// applyBookStatsDeltas adjusts book_stats counters by (field, delta) pairs in one atomic script
// call. Best-effort: on error the hash merely drifts until the next full resync, so log rather
// than fail the caller's request.
func (s *Server) applyBookStatsDeltas(ctx context.Context, fieldDeltas ...any) {
	if err := bookStatsDeltaScript.Run(ctx, s.redis, []string{bookStatsKey}, fieldDeltas...).Err(); err != nil {
		log.Printf("failed to update book stats incrementally: %v", err)
	}
}

// bookStatsRecordInsert counts freshly inserted unlearned rows with discCount discs.
func (s *Server) bookStatsRecordInsert(ctx context.Context, discCount, inserted int) {
	if inserted == 0 {
		return
	}
	s.applyBookStatsDeltas(ctx, bookStatsField(discCount, 0), inserted)
}

// bookStatsRecordSave moves one discCount-disc position from outcome.OldLevel's stats cell to
// newLevel's, when the save updated the row and the levels map to different cells.
func (s *Server) bookStatsRecordSave(ctx context.Context, discCount int, outcome db.SaveOutcome, newLevel int) {
	if !outcome.Updated {
		return
	}

	oldField := bookStatsField(discCount, outcome.OldLevel)
	newField := bookStatsField(discCount, newLevel)
	if oldField == newField {
		return
	}

	s.applyBookStatsDeltas(ctx, oldField, -1, newField, 1)
}

// getBookStats reads the book_stats hash, sorted like statEntries; ok is false when the hash is
// missing (Redis flushed, first boot race) and the caller should fall back to the DB.
func (s *Server) getBookStats(ctx context.Context) (entries []statEntry, ok bool, err error) {
	values, err := s.redis.HGetAll(ctx, bookStatsKey).Result()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get book stats: %w", err)
	}
	if len(values) == 0 {
		return nil, false, nil
	}

	entries = make([]statEntry, 0, len(values))
	for field, value := range values {
		var e statEntry
		if _, err := fmt.Sscanf(field, "%d:%d:%d", &e.Depth, &e.Confidence, &e.DiscCount); err != nil {
			continue
		}
		count, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		if count <= 0 {
			// A cell decremented to zero lingers in the hash until the next full resync.
			continue
		}
		e.Count = count
		entries = append(entries, e.classified())
	}

	sortStatEntries(entries)
	return entries, true, nil
}

// jobFloor returns the lowest disc count the book_stats hash still shows learnable positions for
// (search params below target; depth 0 counts), falling back to book.LeafDiscs when the hash is
// missing. May be stale by one refresh; it is only a lower bound for the partially learned sweep.
func (s *Server) jobFloor(ctx context.Context) int {
	entries, ok, err := s.getBookStats(ctx)
	if err != nil || !ok {
		return book.LeafDiscs
	}

	floor := book.MaxSavableDiscs
	found := false
	for _, e := range entries {
		if e.DiscCount < book.LeafDiscs || e.DiscCount > book.MaxSavableDiscs {
			continue
		}
		learnable := e.Bucket != statBucketLearned
		if learnable && (!found || e.DiscCount < floor) {
			floor = e.DiscCount
			found = true
		}
	}

	return floor
}

// recordJobCompletion increments workerID's positions-computed counter.
func (s *Server) recordJobCompletion(ctx context.Context, workerID string) error {
	pipe := s.redis.TxPipeline()
	pipe.HIncrBy(ctx, workerKey(workerID), workerFieldPositionsComputed, 1)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to record job completion: %w", err)
	}
	return nil
}

// heartbeat records workerID as active and refreshes its claim on position ("" for none), but never
// another worker's claim. GET-compare-EXPIRE is not atomic; the worst case of the race is one
// duplicated evaluation, which is acceptable.
func (s *Server) heartbeat(ctx context.Context, workerID, hostname, gitCommit, position string) error {
	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, workerKey(workerID),
		workerFieldHostname, hostname,
		workerFieldGitCommit, gitCommit,
		workerFieldLastActive, time.Now().Format(time.RFC3339Nano),
	)
	pipe.Expire(ctx, workerKey(workerID), claimTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}

	if position == "" {
		return nil
	}

	owner, err := s.redis.Get(ctx, claimKey(position)).Result()
	if err == redis.Nil || (err == nil && owner != workerID) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to refresh worker claim: %w", err)
	}

	if err := s.redis.Expire(ctx, claimKey(position), claimTTL).Err(); err != nil {
		return fmt.Errorf("failed to refresh worker claim: %w", err)
	}

	return nil
}

// workerInfo describes one worker, as listed by GET /api/workers.
type workerInfo struct {
	ID                string
	Hostname          string
	GitCommit         string
	PositionsComputed int
	LastActive        time.Time
}

// listWorkers returns active workers ordered by positions computed (most first), ties broken by last-active.
func (s *Server) listWorkers(ctx context.Context) ([]workerInfo, error) {
	var workers []workerInfo

	iter := s.redis.Scan(ctx, 0, "worker:*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		values, err := s.redis.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to get worker info: %w", err)
		}
		if len(values) == 0 {
			// Expired between SCAN and HGETALL.
			continue
		}

		positionsComputed, _ := strconv.Atoi(values[workerFieldPositionsComputed])
		lastActive, _ := time.Parse(time.RFC3339Nano, values[workerFieldLastActive])

		workers = append(workers, workerInfo{
			ID:                key[len("worker:"):],
			Hostname:          values[workerFieldHostname],
			GitCommit:         values[workerFieldGitCommit],
			PositionsComputed: positionsComputed,
			LastActive:        lastActive,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan workers: %w", err)
	}

	sort.Slice(workers, func(i, j int) bool {
		if workers[i].PositionsComputed != workers[j].PositionsComputed {
			return workers[i].PositionsComputed > workers[j].PositionsComputed
		}
		return workers[i].LastActive.After(workers[j].LastActive)
	})

	return workers, nil
}

// priorityEntry is the value stored in the priority queue for one pending evaluation job. ConnID is
// the websocket connection that requested it ("" for none); entries whose connection has since
// closed are discarded at dequeue.
type priorityEntry struct {
	Position string `json:"board"`
	Level    int    `json:"level"`
	ConnID   string `json:"conn_id,omitempty"`
}

// priorityQueueKey is a Redis list: positions waiting for priority evaluation (LPUSH to enqueue, RPOP to drain).
const priorityQueueKey = "priority_jobs"

// priorityPendingKey is a Redis set mirroring priorityQueueKey's contents for O(1) duplicate detection.
const priorityPendingKey = "priority_pending"

// analysisResultTTL is how long a priority-computed evaluation lives in the ephemeral cache: long
// enough for the frontend's polling loop to pick it up.
const analysisResultTTL = 30 * time.Minute

// priorityClaimKey is the Redis key marking a claimed job as priority-originated.
func priorityClaimKey(position string) string {
	return "priority_claim:" + position
}

// analysisResultKey is the Redis key holding a priority-computed evaluation as JSON.
func analysisResultKey(position string) string {
	return "analysis:" + position
}

// enqueuePriority adds position to the priority queue unless already pending. Dedupe is by position
// alone: a position pending at another level or for another (possibly dead) connection is not
// re-queued — the frontend re-requests until it gets a result, so drops heal themselves.
func (s *Server) enqueuePriority(ctx context.Context, position string, level int, connID string) error {
	isMember, err := s.redis.SIsMember(ctx, priorityPendingKey, position).Result()
	if err != nil {
		return fmt.Errorf("failed to check priority pending set: %w", err)
	}
	if isMember {
		return nil
	}

	data, err := json.Marshal(priorityEntry{Position: position, Level: level, ConnID: connID})
	if err != nil {
		return fmt.Errorf("failed to marshal priority entry: %w", err)
	}

	pipe := s.redis.TxPipeline()
	pipe.SAdd(ctx, priorityPendingKey, position)
	pipe.LPush(ctx, priorityQueueKey, string(data))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to enqueue priority position: %w", err)
	}
	return nil
}

// dequeuePriority pops the oldest entry (FIFO), removing it from the pending set; ok is false when
// the queue is empty. Nobody is waiting on an entry whose requesting connection has closed, so it
// leaves the queue -- but the position is worth learning anyway, so it is promoted into the book
// instead of being thrown away.
func (s *Server) dequeuePriority(ctx context.Context) (priorityEntry, bool, error) {
	var dropped, promoted int

	for {
		data, err := s.redis.RPop(ctx, priorityQueueKey).Result()
		if err == redis.Nil {
			s.logPriorityDequeue(ctx, "", dropped, promoted)
			return priorityEntry{}, false, nil
		}
		if err != nil {
			return priorityEntry{}, false, fmt.Errorf("failed to dequeue priority position: %w", err)
		}

		var entry priorityEntry
		if jsonErr := json.Unmarshal([]byte(data), &entry); jsonErr != nil {
			// Legacy entries are plain position strings.
			entry = priorityEntry{Position: data, Level: PriorityLevel}
		}

		_ = s.redis.SRem(ctx, priorityPendingKey, entry.Position).Err()

		if entry.ConnID != "" && !s.connLive(ctx, entry.ConnID) {
			dropped++
			if s.promoteToBook(ctx, entry.Position) {
				promoted++
			}
			continue
		}

		s.logPriorityDequeue(ctx, entry.Position, dropped, promoted)
		return entry, true, nil
	}
}

// logPriorityDequeue reports what one drain of the priority queue did and how deep the queue still
// is. Silent when it found nothing and dropped nothing, which is every claim while nobody is
// browsing -- otherwise this would print once per worker poll.
func (s *Server) logPriorityDequeue(ctx context.Context, position string, dropped, promoted int) {
	if position == "" && dropped == 0 {
		return
	}

	depth, err := s.redis.LLen(ctx, priorityQueueKey).Result()
	if err != nil {
		depth = -1
	}

	taken := position
	if taken == "" {
		taken = "nothing"
	}

	log.Printf("priority queue: took %s, dropped %d (%d promoted to the book), %d waiting",
		taken, dropped, promoted, depth)
}

// promoteToBook adds position to the book as an unlearned row, so a priority request whose
// requester disconnected is picked up by the second job tier rather than dropped. Reports whether a
// row was actually created: an out-of-range disc count is refused (see
// Repository.AddPositionsInserted) and a position the book already holds needs no promotion.
func (s *Server) promoteToBook(ctx context.Context, position string) bool {
	parsed, err := othello.ParsePosition(position)
	if err != nil {
		return false
	}

	normalized, err := othello.NewNormalizedPosition(parsed)
	if err != nil {
		return false
	}

	discCount := normalized.CountDiscs()
	if !isSavableDiscCount(discCount) {
		return false
	}

	inserted, err := s.repo.AddPositionsInserted(ctx, []othello.NormalizedPosition{normalized})
	if err != nil {
		log.Printf("failed to promote dropped priority position %s: %v", position, err)
		return false
	}

	s.bookStatsRecordInsert(ctx, discCount, inserted)
	return inserted > 0
}

// setPriorityClaim marks position's claim as priority-originated.
func (s *Server) setPriorityClaim(ctx context.Context, position string) error {
	if err := s.redis.Set(ctx, priorityClaimKey(position), "1", claimTTL).Err(); err != nil {
		return fmt.Errorf("failed to set priority claim: %w", err)
	}
	return nil
}

// consumePriorityClaim atomically reads and deletes position's priority claim marker, reporting
// whether one existed.
func (s *Server) consumePriorityClaim(ctx context.Context, position string) (bool, error) {
	val, err := s.redis.GetDel(ctx, priorityClaimKey(position)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to consume priority claim: %w", err)
	}
	return val != "", nil
}

// setAnalysisResult stores eval in the ephemeral analysis cache; best-effort, so a Redis hiccup
// never fails the submission path.
func (s *Server) setAnalysisResult(ctx context.Context, position string, eval evaluationResponse) {
	data, err := json.Marshal(eval)
	if err != nil {
		return
	}
	_ = s.redis.Set(ctx, analysisResultKey(position), data, analysisResultTTL).Err()
}

// getAnalysisResult retrieves a priority-computed evaluation from the ephemeral cache; ok is false
// on a miss.
func (s *Server) getAnalysisResult(ctx context.Context, position string) (evaluationResponse, bool, error) {
	data, err := s.redis.Get(ctx, analysisResultKey(position)).Bytes()
	if err == redis.Nil {
		return evaluationResponse{}, false, nil
	}
	if err != nil {
		return evaluationResponse{}, false, fmt.Errorf("failed to get analysis result: %w", err)
	}

	var eval evaluationResponse
	if err := json.Unmarshal(data, &eval); err != nil {
		return evaluationResponse{}, false, fmt.Errorf("failed to unmarshal analysis result: %w", err)
	}
	return eval, true, nil
}
