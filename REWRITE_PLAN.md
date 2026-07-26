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

Work through phases in order. Check off each item after it's done and merged.

## Phase 0 — Repo scaffold
- [x] `go.mod`, base folder layout: `cmd/{server,worker,loader}/main.go`,
      `internal/`, `static/`
- [x] `.gitignore` for `db_data/`, `pgn/`, `wthor/`
- [x] `docker-compose.yml` for Postgres
- [x] `migrate` tool wired up (migrations folder + make/script target)
- [x] pre-commit config (fmt, vet/lint, gotestsum)
- [x] CI (if desired) mirroring old `.github/workflows`

`docker-compose.yml`'s Postgres service originally bind-mounted `./db_data`
on the host so dev data survived restarts. Switched to a named Docker
volume (`pgdata`) instead: Postgres in the container still initializes it
`0700` owned by its own uid (999), but since the volume lives in Docker's
own storage rather than the repo tree, that ownership is no longer the
host user's problem — `go build ./...`/`go vet ./...`/`go test ./...` all
recursively walk every directory under the module root to resolve the
`...` pattern, and previously choked with a fatal `permission denied` the
moment that walk reached the root-owned `db_data/`, since Go's tooling
isn't git-aware and doesn't skip arbitrary ignored directories. The named
volume still persists across plain `docker compose down` (no `--volumes`,
what `local.sh` uses on exit) exactly like the bind mount did — verified by
seeding boards, restarting the stack, and confirming the row count
survived. `db_data/` dropped from `.gitignore` since it's never created.

## Phase 1 — Othello core types (`internal/othello`)
- [x] `Board`: white/black bitsets (uint64) + color to move
- [x] move generation / legal moves / apply move / pass handling
- [x] `Game`: sequence of `Board`
- [x] `NormalizedBoard`: wraps `Board`, normalized except color to move is
      preserved — confirm exact normalization algorithm against old
      python/go code before implementing
- [x] unit tests for normalization symmetry, move gen, pass detection

## Phase 2 — Game/file format loaders (`internal/othello` or `internal/loader`)
- [x] wtb (Wthor) file parser → `Game` (there was no Go wtb parser to port;
      ported from the Python implementation instead, the only one that
      existed)
- [x] pgn file parser → `Game` (fixtures copied from the curated Go test
      samples, not the personal `old/pgn` archive, which is unfixtureable
      raw data)
- [x] Othello Quest move-string parser (e.g. `A3B4C5`) → `Game`
- [x] unit tests per format using fixture files

## Phase 3 — Precomputed board set
- [x] Generate/verify the full list of 67 245 `NormalizedBoard`s (positions
      not learned: <12 discs) and embed in source
- [x] unit test asserting the count and uniqueness

## Phase 4 — DB layer (`internal/db` or similar)
- [x] Schema: Board (disc count, search level, evaluation), no best-move
      stored
- [x] Index design for Board lookups (must support disc-count/level ordered
      scans and point lookups by normalized board)
- [x] Migrations via `migrate`
- [x] Repository/query layer with unit tests (against a real Postgres via
      docker, isolated from local dev DB)

## Phase 5 — Edax integration (`internal/edax`)
- [x] Long-running edax subprocess per worker (not one process per position)
- [x] Path to edax binary from env
- [x] Send position + search level, parse evaluation from output
- [x] Confirm edax invocation details (flags, book usage, output format)
      against old python/go integration code before implementing (also
      verified directly against the real edax-reversi binary; book usage
      is a non-issue since `-solve` mode never loads the book at all)
- [x] Graceful shutdown: kill edax process when worker shuts down
- [x] unit tests around request/response parsing (mock the process I/O)

Note: `internal/edax`'s parser/problem-format tests use mocked I/O (fixtures
captured from the real binary) and always run, including in CI. Its
`Process` tests (subprocess spawn/restart/kill) additionally need a real
edax binary via `EDAX_PATH` and skip themselves when it's unset — true
locally, not in CI, which has no edax binary. This is a deliberate choice
(see chat history): CI doesn't build or fetch edax, since the parsing logic
most likely to silently break is already covered by the mocked tests, and
the subprocess plumbing left uncovered in CI is thin `os/exec` code with
low bug surface. Run `EDAX_PATH=... ./test.sh` locally for full coverage of
this package.

## Phase 6 — API server (`cmd/server`, `internal/api`)
- [x] JSON REST API prefixed `api/`
- [x] `GET` job endpoint: atomic job claim (lowest disc count, then lowest
      learn level; skip <12 discs and >30 empties)
- [x] `POST` job result endpoint
- [x] `GET` evaluation-by-(non-normalized)-Board endpoint
- [x] Worker heartbeat endpoint + reaping: jobs from dead workers become
      claimable again
- [x] Stats endpoint: single response with move-counts per (level, disc
      count) cell, indexed appropriately for a large Board table
- [x] unit tests per handler; integration tests against dockerized Postgres

Notes on decisions made along the way (none spelled out in `prompt.md`;
confirmed with the user during implementation):
- The "skip <12 discs and >30 empties" bound resolves, against `old/`'s
  `MaxBookSavableDiscs`/`MAX_SAVABLE_DISCS` (both `30`), to a plain
  disc-count range: boards are only learnable with 12-30 discs.
- `api.TargetLevel(discCount)` is the single place deciding what level a
  board should be learned to. It always returns 24 today regardless of
  discCount — a placeholder kept as a real function (not a constant) so a
  future per-disc-count scheme is a one-function change.
- Job claims and worker identity are **not** stored in the `boards` table or
  a new Postgres table — extra columns/rows were rejected for a table
  expected to grow large. Instead Redis holds this ephemeral state: a
  `claim:<board string>` key (worker ID, TTL) per in-flight job, and a
  `worker:<id>` key (board string, same TTL) so a worker's periodic
  heartbeat can find and refresh its own claim. Expiry alone does the
  reaping — no sweep/cron needed. This adds a new dependency
  (`redis/go-redis`) and a `redis` service in both docker-compose files and
  CI.
- There's no worker registration endpoint. A worker mints its own ID at
  startup and sends it on every request; the heartbeat endpoint is a no-op
  for a worker with no active claim rather than an error.

## Phase 7 — Startup minimax cache (`internal/book`)
- [x] On server start, minimax all positions with <12 discs from the set of
      12-disc evaluations, build in-memory map
- [x] Recompute this map whenever a 12-disc evaluation is saved
- [x] unit tests for the minimax/backfill logic

Note: this header originally said "<11 discs", which would have left
exactly-11-disc boards uncovered by both the DB (which only ever stores
>=12 discs) and this cache. Confirmed with the user: the cache covers every
disc count below the DB's own floor, i.e. 4-11 discs, matching Phase 3's
framing ("positions not learned: <12 discs").

`internal/api`'s `GET /api/boards` now falls back to this cache when a
board isn't in the DB, returning `"source": "minimax"` (vs `"edax"` for a
direct DB result) — not a Phase 7 checklist item as written, but confirmed
with the user as in-scope, since otherwise the cache would have had no
external consumer yet.

## Phase 8 — Worker (`cmd/worker`, `internal/worker`)
- [x] Loop: fetch Job (Board + level) from API → hand to long-running edax →
      await evaluation → POST Job+result back to API
- [x] Periodic heartbeat to server
- [x] Clean shutdown (edax process + in-flight job handling)
- [x] unit tests with a fake API client and fake edax

`internal/worker.Client` talks to the Phase 6 API (own JSON wire types,
matching but not sharing `internal/api`'s unexported ones); `Worker` depends
on small `apiClient`/`evaluator` interfaces so `internal/worker/worker_test.go`
can inject hand-rolled fakes per the checklist, while
`internal/worker/client_test.go` separately verifies `Client` against a real
`api.Server` (via `httptest`) so the wire format itself is checked, not just
an assumed contract.

Clean shutdown: canceling the worker's context can't interrupt a blocked
edax evaluation (edax's process I/O doesn't take a context), so
`cmd/worker` also closes the edax process directly, concurrently, on
`ctx.Done()` — verified manually against the real edax binary: `SIGTERM`
sent to the worker mid-search killed the edax subprocess within ~200ms.
(An earlier scare during this manual testing — an edax process apparently
outliving its worker — turned out to be `go run`'s wrapper process not
forwarding `SIGTERM` to the binary it execs, not a bug in this code; the
compiled binary shuts down its edax subprocess correctly.)

## Phase 9 — Frontend (`internal/web`, `static/`)
- [x] Go `html/template`-based admin site, left sidebar for page nav
- [x] Main page: board rendering with per-square evaluation; small colored
      dot (color = side to move) when evaluation missing; evaluation number
      (same color convention) when present; pass handling; right-click on
      board undoes last move
- [x] Stats page: table of move-counts, rows = disc count, cols = level;
      zero-count cells render empty; backed by the Phase 6 stats endpoint
- [x] websocket wiring (confirm what it's used for against old code)
- [x] unit tests for handlers/template rendering where practical

Decisions made with the user during implementation (none spelled out in
`prompt.md`):
- **WASM fallback evaluator skipped.** Old code ran a bundled Rust→WASM
  Othello engine client-side to show an instant approximate score (styled
  grey) for child positions while waiting on the real edax/DB evaluation.
  `prompt.md`'s own description of the main page doesn't mention this — dot
  when missing, number when present — so it was left out to avoid a
  Rust/wasm-pack build pipeline for a cosmetic feature. **Revisit later**:
  worth reconsidering if the plain "dot until the real evaluation arrives"
  UX feels too slow in practice.
- **Clients page included**, even though it's a 3rd page not in `prompt.md`
  (which only anticipates "other pages may be added later"). This required
  two additions beyond the old worker-registration model established in
  Phase 6 (workers still don't register — no `POST /api/workers/register`):
  - Worker heartbeats now carry `hostname` and `git_commit`; a worker
    determines its own hostname (`os.Hostname`) and best-effort git commit
    (`git rev-parse HEAD`, falling back to `"unknown"` for a deployed binary
    without a `.git` checkout) rather than the server assigning identity.
  - `worker:<id>`'s Redis value changed from a plain string (the claimed
    board, for claim-TTL refresh) to a hash, adding `hostname`,
    `git_commit`, `positions_computed` (via `HINCRBY` on each submitted job
    result), and `last_active` (nanosecond-precision RFC3339, so two
    heartbeats landing in the same wall-clock second still sort correctly)
    alongside the existing `claimed_board` field. New `GET /api/workers`
    scans and returns these, most-recently-active first.
- **Websocket** (`GET /ws`, in `internal/api` since it shares the same
  repo/cache/redis dependencies as the REST handlers, not a separate
  package) is confirmed to be exactly what old code used it for: batched,
  low-latency evaluation lookups for the children of whatever board is on
  screen, not live/on-demand edax computation. Old code's
  `lookupPositions` behind the websocket was already just a DB batch
  `SELECT`; this port does the same DB lookup plus the Phase 7 minimax
  cache fallback that `GET /api/boards` also uses.
- Best-move circle highlighting (border around the best child once *all*
  of a position's children have a known evaluation) isn't in `prompt.md`
  either, but was kept as a small, cheap port of existing UX rather than a
  scope question worth asking about.
- The client-side board (`static/board.js`) reimplements
  `internal/othello`'s bitboard move generation and normalization in
  JavaScript, since the browser needs to simulate moves locally for
  instant feedback without a server round trip per click — this mirrors
  what old `game.js` already did. Verified byte-for-byte against the real
  Go implementation (not just old code): the starting position's
  `toString()` output and a first-move child's normalized form were
  checked against actual `internal/othello` output captured earlier in
  this phase's work.
- No browser was available for a literal visual/click-through smoke test
  (the `claude-in-chrome` skill invocation was declined). Verified instead
  via: a live server + worker + real edax computing actual evaluations
  end-to-end; `node --check` on all three JS files; the board bitboard/
  normalization logic run for real in Node and compared against known-good
  Go output; the websocket protocol driven from Node's native `WebSocket`
  against the live server using the exact message shape `board.js` sends;
  and every page/static-asset route checked for correct status and
  content-type. This is not a substitute for an actual visual check —
  flag if one is wanted before relying on this further.

## Phase 10 — Loader (`cmd/loader`)
- [x] Command to import files (wtb/pgn/move-string) as `Game`s and extract
      `NormalizedBoard`s to add to DB
- [x] Adding boards never updates/removes existing DB rows; learning only
      updates existing rows, never adds/removes
- [x] unit tests for the extraction logic (I/O-free)

`internal/loader.ExtractBoards` takes every board on each imported game's
played line and, since `old/`'s two implementations diverge here (Python
`load_pgn.py` adds one-ply children of played-line boards, `load_wthor.py`
doesn't, and `prompt.md` doesn't mention children at all), also adds every
legal one-ply child of those boards — confirmed with the user. A board is
kept only if it has a legal move and its disc count is in
`[book.LeafDiscs, book.MaxSavableDiscs]` (12-30), the same range
`ListLearnable`/job-claiming and the Phase 7 cache already care about;
`book.MaxSavableDiscs` (30) was pulled out of `internal/api`'s previously
unexported `maxLearnableDiscs` into `internal/book` so both packages (and
now `internal/loader`) share one constant instead of the bound being
duplicated. `AddBoards` itself (Phase 4) already provides the
never-updates/never-removes semantics; the loader just calls it.

`cmd/loader` gained three subcommands alongside `seed`: `load-wtb
<files...>`, `load-pgn <files...>`, `load-oq <moves>`. This is scoped
smaller than `old/`'s incremental PGN importer (`.flippy/pgn.json`
last-processed-file tracking, folder scanning) — that's listed under
"CLI tools" below for a Phase 11 port/drop decision, not part of this
phase's checklist.

While testing this phase, `./test.sh` (and CI's `go test ./...`)
intermittently hit Postgres deadlocks from concurrent packages calling
`AddBoards` against the same test DB at once (`go test`'s default
cross-package parallelism). Fixed by adding `-p 1` to both `test.sh`'s
`gotestsum` invocation and the CI workflow's `go test`, serializing
packages; within a package, tests already isolate via one transaction each.

## Phase 11 — Final pass
- [ ] Full `pre-commit run -a` + full test suite green
- [ ] Manual smoke test: worker + server + edax end-to-end on a few jobs
- [ ] Manual smoke test: frontend main page + stats page in browser
- [ ] Review "Behavior not covered by prompt.md" section below with user;
      resolve each item to "port" or "drop"; update this plan accordingly

---

## Behavior/features in `old/` not mentioned in `prompt.md`

The old code has two divergent implementations (Go, Python). Each item notes
its origin. For each, decide: port it, or explicitly drop it.

### Contradicts prompt.md (needs a decision, not just a port/drop)
- **Best move storage**: Python DB stores `best_moves` per row; Go
  `Evaluation.Validate` also requires legal best moves. prompt.md says
  "do not store the best move" — confirm this is intentional.
- **Edax process model**: Python spawns a fresh edax subprocess per batch
  request (positions piped via stdin); Go keeps one long-running process,
  restarted only on level change. prompt.md already specifies long-running
  (Go's model) — Python's is superseded, not a gap.
- **Minimum learn level**: both old impls also enforce `level >= 16`
  (Python `MIN_LEARN_LEVEL`, Go `Evaluation.Validate`) as a floor distinct
  from the disc-count (<12) / empties (>30) rules in prompt.md.

### API
- Client/worker registry: `POST /api/learn-clients/register` (hostname +
  git commit → UUID `client_id` sent via `X-Client-Id`; re-registers on
  401), heartbeat endpoint, and `GET /api/learn-clients` listing worker
  status (used to avoid double-assigning a job to a worker already on it).
- `GET /api/positions/stats` (book stats) and `GET /version`.
- Auth: `X-Token` header OR HTTP Basic Auth (middleware), separate Basic
  Auth guarding HTML admin pages.
- WebSocket endpoint for on-demand position evaluation (see Frontend).
- Job cache prefetch/refresh strategy backed by Redis (list of pending
  jobs, single-flight refill, per disc-count/level counters) — an
  implementation detail, but the *counts-cache* concept may be worth
  keeping for the stats endpoint's performance.
- `Evaluation.Validate`: level ≥ 16, depth 0–60, confidence restricted to
  `{73,87,95,98,99,100}`, score in -64..64.

### Frontend (web)
- **Clients page** (3rd page, not in spec): table of active workers
  (hostname, git commit, positions computed, last-active), polled ~1/s.
- Stats page auto-refreshes (~1/s), not just static render.
- Client-side WASM fallback evaluator: while waiting on authoritative
  edax result for a child position, browser runs a bundled Rust→WASM
  Othello engine for an instant approximate score, shown visually
  distinct (greyed) from server evaluations.
- Right-click anywhere on the board (not just a button) triggers undo.
- WebSocket protocol for on-demand evals: client requests evals for
  visible children not yet cached; auto-reconnects after 1s on close.

### Desktop GUI (Go raylib `cmd/gui`, Python pygame `commands/gui.py`)
Not a web page — separate desktop binary/app, absent from prompt.md
entirely. Decide whether to keep as a dev/analysis tool.
- Go: single free-play board view with keyboard shortcuts (`D` log board
  string, `N` restart, `E` toggle background edax eval, `Space` toggle
  depth display); per-move score + best-move ring overlay.
- Python: 5 modes — `game` (play), `pgn` (step through a PGN with
  alt-move exploration, score graph, board flip), `evaluate` (iterative
  deepening on a free position), `frequency` (move-frequency stats from
  local PGN archive), `watch` (screen-scrapes a live game off a website
  via pixel matching).

### CLI tools
- `load_pgn` / `book load-pgn`: incremental PGN importer that tracks
  last-processed filename in `.flippy/pgn.json` so reruns only pick up
  new files (both Go and Python have this).
- `book load-wthor`: bulk WTHOR archive loader (Python).
- `book validate` / `validate_db`: recomputes disc count from board bits
  and re-checks evaluation invariants, flags mismatches (Python).
- `pgn_analyzer`: annotates a PGN move-by-move against edax evals,
  flagging non-best moves (Python).
- `pgn_organizer`: sorts PGNs into date folders by variant; downloads
  games from playok.com and Othello Quest (reverse-engineered
  websocket protocol) (Python).
- `recent_games`: prints last N games with result summary (Python).
- `show_board`: prints ASCII board from a board string and exits (Go).
- `learn_client`: standalone worker CLI, separate from an in-process
  server-embedded worker (Go) — confirms worker ships as its own binary.
- `dummy`: placeholder, no functionality (Go) — drop.

### Edax integration
- Output parsing is column/regex based: confidence from `@NN%` suffix,
  best-move fields at a fixed byte offset; skips table borders/headers.
- Positions with no legal moves are never sent to edax (it crashes on
  them) — pass/game-end positions handled before invoking edax.
- `FLIPPY_EDAX_VERBOSE` debug flag dumps command/cwd/stdin/stdout.
- In-memory eval cache in front of persistent DB, upsert-if-deeper.

### Othello types/parsing
- XOT games are metadata-only in old code: `IsXot` flag is stored but
  there is no special start-position or legality logic — confirm the
  rewrite doesn't need real XOT support either, or scope it in.
- Pass-move encodings accepted in parsers: `"--"`, `"ps"`, `"pa"` (Go and
  Python both).
- PGN parser auto-inserts a pass move when a move is illegal, and errors
  on missing Site/White/Black/Result or unknown `Variant`; Date assumes
  Europe/Stockholm timezone.
- WTHOR binary format specifics: 16-byte header, 68-byte records, move
  byte = `row*10+col` (1-indexed).
- `Position.random(disc_count)` test helper: generates a random legal
  position with N discs (Python).

### Config/Infra
- All config via `.env` + required env vars that hard-exit if missing
  (host/port, DB/Redis URLs, auth creds/token, edax path, static dir).
- `PROJECT_ROOT` placeholder substitution in path-like env vars.
- `LOG_LEVEL` env var controlling log verbosity.

### Other
- `wasm/`: self-contained Rust→WASM Othello engine (bitboard + alpha-beta
  bot) built for the client-side fallback evaluator above — otherwise
  unused by the backend.
- `archive_book.sh`: ops script that `pg_dump`s the evaluation table to a
  timestamped, gzipped file for backup.
