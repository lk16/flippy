# Implementation plan: Redis simplification, priority-pipeline fixes, logging, cleanup

Instructions for implementing an agreed set of changes. Work through the steps **in order** — later
steps assume earlier ones are done. Each step is one commit (or a few coherent commits). Before
starting, read `CLAUDE.md` and the docs it links, especially `docs/testing.md` (how to lint and
test; edax-dependent Process tests run locally only, never in CI).

Ground rules:

- Keep all existing tests. Adjusting them for renames/moves or simplifying them is fine as long as
  the tested functionality is unchanged. Add tests where a step says so.
- After every step: `gofmt`, lint, and the full test suite must pass.
- Steps 1–5 change behavior and must each be reviewable on their own. Steps 6–8 are mechanical.

## Step 1: Single-job worker, no Lua scripts, no worker-claims set

Current state (`internal/api/claims.go`, `internal/worker/`): workers claim batches of jobs
(`defaultJobBatchSize = 10`) into a prefetch channel; the server tracks each worker's claims in a
`worker_claims:<id>` Redis set so the heartbeat can refresh every claim's TTL via
`refreshClaimsScript` (Lua); `releaseClaimScript` (Lua) guards release-only-if-owner.

Target state:

- **Worker claims exactly one job at a time.** Remove the prefetch channel, `runRefill`,
  `releaseQueuedJobs`, and the batch-size config. `Run` keeps the heartbeat and stats goroutines;
  the main loop is: claim one job → evaluate → submit → repeat (sleep the existing poll interval
  when no job is available). Track the currently claimed board in a mutex- or atomic-protected
  field so the heartbeat can report it and shutdown can release it (at most one).
- **Heartbeat carries the claimed board.** Add an optional `board` field to the heartbeat request.
  The server refreshes that claim key's TTL only if its value equals the worker ID (plain GET,
  compare, EXPIRE — no Lua; the race is benign).
- **Delete both Lua scripts and the `worker_claims:<id>` set** and everything touching them.
  `releaseClaim` becomes GET, compare, DEL. Worst case of the non-atomic release/refresh races is
  one duplicated evaluation, which is acceptable; note this in a short comment.
- **Simplify the jobs endpoint to a single job.** The worker is its only client. `claimJobs`
  becomes claim-one: drain one live priority entry first, else scan `ListLearnable` candidates
  from the job floor. Keep `jobCandidateBatch` for the DB fetch size.

Keep: `claim:<board>` SetNX with `claimTTL`, the `worker:<id>` hash and its TTL refresh, release on
worker shutdown (this fixed a real starvation problem — do not regress it).

Tests: adapt existing claims/worker tests; add coverage for heartbeat-refreshes-claim-TTL and for
release/refresh not touching a claim owned by another worker.

## Step 2 (was 3+4): `book_stats` hash replaces the stats cache and job floor

Current state: `GET /api/stats` caches its JSON in the `stats` key (60s TTL + explicit
invalidation on every save); `claimJobs` caches the lowest learnable disc count in
`job_floor_disc_count` (10m TTL, advanced on claim).

Target state:

- A server goroutine rebuilds a Redis hash `book_stats` at startup and every 60s: run
  `repo.Stats()` (the existing GROUP BY over disc count/level), convert through the existing
  level→(depth, confidence) merge (`statEntries` logic; level 0 → depth 0, confidence 0), then
  write fields `"<depth>:<confidence>:<discs>"` → position count into a temp key and atomically
  swap with `RENAME`. The goroutine stops with server shutdown.
- `GET /api/stats` reads the hash and serves the same JSON shape as today, sorted as today. If the
  hash is missing (Redis flushed, first boot race), fall back to querying the DB directly.
- The job floor is **derived** from the hash: the lowest disc count that still has learnable
  positions, i.e. a field whose (depth, confidence) is below what
  `edax.SearchParams(discs, TargetLevel(discs))` requires (depth 0 counts as learnable). Fallback
  to `book.LeafDiscs` when the hash is missing. Floor staleness up to one refresh period is
  acceptable: the floor is only a lower bound for the `ListLearnable` scan.
- Delete: `statsKey`, `statsTTL`, `getCachedStats`, `setCachedStats`, `invalidateStatsCache` (and
  its call in `handleSubmitJobResult`), `jobFloorKey`, `jobFloorTTL`, `getJobFloor`, `setJobFloor`
  and the floor-advancing logic in `claimJobs`.

Tests: hash rebuild produces the expected fields from known DB rows; stats endpoint serves from
the hash and falls back to the DB; floor derivation picks the lowest learnable disc count and the
fallback works.

## Step 3 (was 5a): enforce book quality on every submission

`isBookQuality` (level ≥ `TargetLevel(discCount)`, or final) is currently checked only for
priority-originated results; the non-priority path in `handleSubmitJobResult` saves whatever level
a worker submits. Enforce it API-wide: any submission failing `isBookQuality` is accepted (200,
still cached ephemerally, claim released, completion counted) but **never** written to the DB,
priority or not.

Tests: both paths — a priority result below target is not saved; a non-priority result below
target is not saved; at-target and final results are saved.

## Step 4 (was 5b): schedule ineligible-but-savable boards for future learning

When a priority result is book-ineligible **but** `discCount <= book.MaxSavableDiscs`, the board
currently may have no DB row at all, making it invisible to the book learner. Change: in that
case, insert the board via `AddBoards` (which creates a row with an empty evaluation — level 0,
depth 0, confidence 0, score 0) so `ListLearnable` picks it up later. This must be insert-if-absent
only: an existing row with a real evaluation must never be downgraded. Verify `AddBoards` has
ON CONFLICT DO NOTHING semantics (or equivalent) before relying on it.

Tests: ineligible priority result on an unknown savable board creates an empty-evaluation row;
on a board with an existing deeper evaluation, the row is untouched; boards above
`MaxSavableDiscs` get no row.

## Step 5 (was 5c): drop priority work for disconnected clients

Priority analysis requests arrive over the websocket (`handleAnalyzeRequest`). If the user
navigates away, queued positions are computed for nothing.

- Give each websocket connection an ID. Tag every `priorityEntry` with it.
- Track live connection IDs in memory on the server (single-process deployment; in-memory is
  fine). Remove the ID on disconnect.
- `dequeuePriority` lazily discards entries whose connection ID is no longer live (also `SRem`
  them from `priority_pending`) instead of rewriting the queue on disconnect.
- Jobs already claimed by a worker run to completion; that is accepted (queue depth dwarfs
  in-flight count, and the result lands harmlessly in the ephemeral cache).

Tests: entries from a dead connection are skipped at dequeue and removed from the pending set;
entries from live connections still come through; a board queued by two connections survives if
either is alive (or document and test the chosen semantics if simpler — dedupe is by board only).

## Step 6: replace slog with plain log

Replace all `log/slog` usage (server, worker, loader) with the standard `log` package and plain,
human-readable messages. Request-duration logging (HTTP middleware) prints milliseconds with one
decimal, e.g. `GET /api/stats 200 12.3ms`. No structured key=value output anywhere.

## Step 7: consolidate files and functions

The `internal/api` package is ~3.5k lines over 18 files for handlers + Redis coordination + a
websocket. After steps 1–5 have deleted their share, merge what remains; suggested targets (adapt
to what still exists):

- `redis.go` + `claims.go` + `priority.go` → one Redis-coordination file.
- `level.go` → `jobs.go`; `types.go` → `handlers.go`. Merge the corresponding test files.
- Inline single-use helper functions where the inlined version is clearer.

Functionality must be identical — this step is moves, merges, and inlining only. Keep all tests
(imports/renames may change).

## Step 8: cut comments to the minimum

Comments throughout the repo are far too long: multi-paragraph narratives on short functions,
history-telling, and change descriptions. Rewrite to: one line per exported symbol, plus only
constraints the code cannot express (e.g. "edax crashes on a position with no legal move" stays;
design essays go). Never describe a change or the past. Apply across `internal/`, `cmd/`, and
`wasm/`. Add a short rule to `docs/style.md` codifying this style. Steps 7 and 8 may be done as
one pass.

## After each step

Run lint and tests per `docs/testing.md`. Update `docs/` where a step invalidates documented
behavior (e.g. the stats cache and claim mechanics in `docs/project.md`, if described there).
Delete this file in the final commit.
