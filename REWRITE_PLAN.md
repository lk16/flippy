# Flippy Rewrite Plan

Full rewrite of flippy (Othello tools: DB of pre-computed openings, webserver
with REST API + websocket + frontend, worker that pre-computes positions via
edax) in Go. Source of truth for requirements: `prompt.md`.

Rules while executing this plan:
- Never assume behavior; confirm against `old/` code first, then ask the user.
- Do not reference or import anything from `old/` — read-only reference.
- Run `pre-commit run -a` and all tests before each commit.
- All backend code except `cmd/*/main.go` entrypoints needs unit tests.
- Prefer returning errors over panicking; panics are only for internal
  invariant violations that indicate a bug, never for expected failure
  conditions (invalid input, invalid moves, I/O errors, etc).
- Don't refer to `old/` or its code as "legacy" (or at all) in generated
  code, comments, or commit messages — it's read-only reference material for
  us, not part of this codebase's history.
- Run tests with code coverage. Coverage doesn't prove behavior is correct,
  but uncovered lines outside `cmd/*/main.go` entrypoints flag missing
  tests.

## Implemented (Phases 0-10)

- **Repo scaffold**: `go.mod`, `cmd/{server,worker,loader}`, `internal/`,
  `static/`; `docker-compose.yml` (Postgres + Redis, named `pgdata` volume);
  `migrate`-based migrations; pre-commit (fmt/vet/lint/gotestsum); CI
  (fmt, golangci-lint, test workflows).
- **Othello core** (`internal/othello`): bitboard `Board` (black/white
  `uint64` + turn), move generation/apply/pass handling, `Game` (move
  history), `NormalizedBoard` (canonical form across the 8 symmetries, turn
  preserved), wtb/pgn/Othello-Quest-move-string parsers, and the
  precomputed set of 67 239 `NormalizedBoard`s with <12 discs, embedded in
  source.
- **DB layer** (`internal/db`): `boards` table storing only disc count,
  search level, and evaluation (no best-move); `Repository` with
  `AddBoards` (insert-only), `GetBoard`, `SaveEvaluation`
  (upsert-only-if-deeper), `ListLearnable`, `EvaluatedBoards`, `Stats` —
  indexed for both disc-count/level ordered scans and point lookups.
- **Edax integration** (`internal/edax`): one long-running edax subprocess
  per worker (not one per position), `-solve` problem format, output
  parsing, graceful subprocess shutdown.
- **API server** (`cmd/server`, `internal/api`): REST endpoints for job
  claim/result, board evaluation lookup (DB with minimax-cache fallback),
  worker heartbeat + dead-worker reaping, stats, and a websocket for
  batched child-position lookups. Job claims and worker identity live only
  in Redis (TTL keys), never in Postgres.
- **Startup minimax cache** (`internal/book`): backfills every board below
  12 discs from the 12-disc evaluations; rebuilt whenever a 12-disc
  evaluation is saved.
- **Worker** (`cmd/worker`, `internal/worker`): fetch job → edax → submit
  result loop, periodic heartbeat, clean shutdown that also kills the edax
  subprocess.
- **Frontend** (`internal/web`, `static/`): Go `html/template` admin site —
  board page (per-square evaluation, pass handling, right-click undo,
  best-move circle), stats page (move-counts per disc-count/level,
  auto-refreshing), clients page (active workers, auto-refreshing); a
  websocket-driven client prefetches child-position evaluations.
- **Loader** (`cmd/loader`): `seed`/`load`/`load-oq` subcommands; extracts
  every played-line board plus its one-ply children; add-only semantics
  (loading never updates or removes existing rows).

### Decisions worth flagging (none spelled out in `prompt.md`)

- 6 no-legal-move positions were removed from the precomputed 12-disc set —
  edax can't evaluate them and they'd sit unlearnable forever; matches the
  save-eligibility rule already used elsewhere.
- `api.TargetLevel`: the 12-disc leaves (source for all backfilled values)
  are learned to level 24; everything else only to level 16.
- Job claims/worker identity are Redis-only (TTL, no sweep/cron needed) —
  keeps the `boards` table, expected to grow large, free of ephemeral
  columns.
- No worker registration endpoint — a worker mints its own ID and sends it
  on every request.
- The minimax cache covers discs 4-11 (everything below the DB's own
  12-disc floor).
- The client-side WASM fallback evaluator (instant approximate score while
  waiting on a real evaluation) was dropped — not in `prompt.md`, and
  avoids a Rust/wasm-pack build pipeline for a cosmetic feature. Revisit if
  the plain "dot until the real evaluation arrives" UX feels too slow.
- The clients page (a 3rd page, beyond `prompt.md`'s two) required adding
  `hostname`/`git_commit` to heartbeats and switching a worker's Redis value
  from a plain string to a hash.
- `static/board.js` reimplements `internal/othello`'s bitboard move
  generation/normalization in JS (verified byte-for-byte against Go output)
  since the browser needs instant local move simulation.
- `test.sh`/CI run `go test -p 1` (serialized packages) — concurrent
  packages calling `AddBoards` against the same test DB caused Postgres
  deadlocks under default parallelism.
- `internal/edax`'s subprocess (`Process`) tests need a real edax binary via
  `EDAX_PATH` and skip themselves in CI (no edax fetched there); parser/
  problem-format tests use captured-fixture mocks and always run.

## Phase 11 — Final pass

- [x] Full `pre-commit run -a` + full test suite green
- [ ] Manual smoke test: worker + server + edax end-to-end on a few jobs
- [ ] Manual smoke test: frontend main page + stats page in browser
- [ ] Review "Potential future steps" below with user; act on anything worth
      pulling forward, otherwise leave as future work

---

## Potential future steps

Behavior present in `old/` (or otherwise identified) that was deliberately
left out of this rewrite. None of it is required by `prompt.md` — revisit
only if a real need for it comes up.

- **Auth**: no request auth at all yet (old had `X-Token`/HTTP Basic Auth
  middleware for the API, and separate Basic Auth for the HTML admin pages).
- **`GET /version`**: no version/build-info endpoint.
- **Job cache prefetch**: `ListLearnable` queries Postgres directly on every
  job claim; old kept a Redis-backed prefetch/refill cache of pending jobs
  plus per disc-count/level counters. Only worth it if claim latency
  becomes a problem.
- **Stricter evaluation validation**: submitted job results check
  level/depth/score bounds, but not a confidence enum
  (`{73,87,95,98,99,100}`) or an explicit level floor (`TargetLevel` itself
  never assigns below 16, so today the floor is enforced by construction,
  not validated on input).
- **PGN illegal-move tolerance**: old auto-inserted a pass when a recorded
  move was illegal, recovering from bad data; this parser errors instead.
  Only matters if a real-world PGN file turns up that needs it (mirrors the
  Result/rating tolerance already added).
- **Desktop GUI**: no `cmd/gui` (Go raylib) or pygame equivalent — free-play
  board view, PGN stepping with alt-move exploration, iterative-deepening
  eval, move-frequency stats, live-game screen-scraping are all absent.
- **CLI tools**: incremental PGN folder-watch importer (`.flippy/pgn.json`
  last-processed tracking), `book validate`/`validate_db` (recompute disc
  count and re-check evaluation invariants), `pgn_analyzer` (annotate a PGN
  against edax evals), `pgn_organizer` (sort PGNs by date/variant, download
  from playok.com/Othello Quest), `recent_games`, `show_board`.
- **`FLIPPY_EDAX_VERBOSE`**: no debug flag to dump edax command/cwd/stdin/
  stdout.
- **Config/infra niceties**: no `PROJECT_ROOT` placeholder substitution in
  env vars, no `LOG_LEVEL` verbosity control.
- **JS test coverage**: `static/board.js`/`stats.js`/`clients.js` have no
  automated test suite — verified so far only via `node --check` (syntax)
  and one-off manual comparisons against Go output. Worth adding real JS
  unit tests (e.g. Node's built-in test runner) for `OthelloBoard`'s
  bitboard move generation/normalization and the websocket client's
  reconnect/queueing logic, so this logic doesn't keep relying on manual
  verification.
