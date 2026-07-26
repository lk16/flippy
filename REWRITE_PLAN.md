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
- [ ] JSON REST API prefixed `api/`
- [ ] `GET` job endpoint: atomic job claim (lowest disc count, then lowest
      learn level; skip <12 discs and >30 empties)
- [ ] `POST` job result endpoint
- [ ] `GET` evaluation-by-(non-normalized)-Board endpoint
- [ ] Worker heartbeat endpoint + reaping: jobs from dead workers become
      claimable again
- [ ] Stats endpoint: single response with move-counts per (level, disc
      count) cell, indexed appropriately for a large Board table
- [ ] unit tests per handler; integration tests against dockerized Postgres

## Phase 7 — Startup minimax cache
- [ ] On server start, minimax all positions with <11 discs from the set of
      12-disc evaluations, build in-memory map
- [ ] Recompute this map whenever a 12-disc evaluation is saved
- [ ] unit tests for the minimax/backfill logic

## Phase 8 — Worker (`cmd/worker`)
- [ ] Loop: fetch Job (Board + level) from API → hand to long-running edax →
      await evaluation → POST Job+result back to API
- [ ] Periodic heartbeat to server
- [ ] Clean shutdown (edax process + in-flight job handling)
- [ ] unit tests with a fake API client and fake edax

## Phase 9 — Frontend (`internal/web`, `static/`)
- [ ] Go `html/template`-based admin site, left sidebar for page nav
- [ ] Main page: board rendering with per-square evaluation; small colored
      dot (color = side to move) when evaluation missing; evaluation number
      (same color convention) when present; pass handling; right-click on
      board undoes last move
- [ ] Stats page: table of move-counts, rows = disc count, cols = level;
      zero-count cells render empty; backed by the Phase 6 stats endpoint
- [ ] websocket wiring (confirm what it's used for against old code)
- [ ] unit tests for handlers/template rendering where practical

## Phase 10 — Loader (`cmd/loader`)
- [ ] Command to import files (wtb/pgn/move-string) as `Game`s and extract
      `NormalizedBoard`s to add to DB
- [ ] Adding boards never updates/removes existing DB rows; learning only
      updates existing rows, never adds/removes
- [ ] unit tests for the extraction logic (I/O-free)

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
