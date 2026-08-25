# Project

Flippy builds an Othello opening book: positions are evaluated by edax,
stored in Postgres, and browsed via a web frontend.

## Components

- `cmd/server` (`internal/api`, `internal/web`) — REST API + websocket +
  HTML admin pages (game/analysis, stats, clients). Serves evaluations from
  the DB, with an in-memory minimax cache (`internal/book`) that backfills
  every <12-disc position from the 12-disc evaluations; a leaf-disc save
  bumps the Redis `book:version` counter and every replica polls it and
  rebuilds. Endpoints workers
  mutate book state through require the `FLIPPY_WORKER_TOKEN` bearer
  token; browsing endpoints and the pages are open. `/healthz` answers as
  soon as the listener is up; `/readyz` only once the cache has built and
  Postgres/Redis ping; `/version` reports the build commit.
- `cmd/worker` (`internal/worker`, `internal/edax`) — claims one job at a
  time from the server, evaluates it with one long-running edax
  subprocess, submits the result, heartbeats (each heartbeat refreshes the
  claim's TTL). Job claims and worker identity live only in Redis
  (TTL keys), never in Postgres.
- `cmd/loader` (`internal/loader`) — subcommands `seed` (precomputed
  12-disc set, embedded in source), `load` (PGN files), `load-oq`
  (Othello Quest move strings). Add-only: never updates or removes rows.
- `internal/othello` — bitboard `Position`/`Game`, move generation,
  `NormalizedPosition` (canonical across the 8 symmetries), wtb/PGN/Othello
  Quest parsers. A `Position` is the mover-relative `(player, opponent)`
  pair edax evaluates, with no color to move; disc colors exist only where
  a game is displayed (`static/board.js`) or described (PGN metadata). The
  HTTP API and the `boards` table still say "board" for a position.
  `static/board.js` reimplements the bitboard logic in JS (verified
  byte-for-byte against Go output) so the browser can simulate moves
  locally.

## Stack

- Go (version in `go.mod`); deps: pgx, go-redis, coder/websocket, testify, godotenv
- Postgres — single `boards` table (position, disc count, level, score;
  position is the 128-bit `player||opponent` big-endian pair, depth and
  confidence follow from disc count + level, see
  `internal/edax.SearchParams`); migrations via golang-migrate
  (`migrations/`), one-shot operator SQL in `scripts/`
- Redis — job claims, worker heartbeats, priority queue, ephemeral
  analysis results, the shared job candidate buffer workers pop from
  (`job:buffer`, entries tagged with the tier that buffered them, kept
  5000 deep by a background top-up on the server — every replica polls
  the depth, one refills when it falls to a fifth of that, so a claim
  pops a candidate instead of waiting on the scan that finds one;
  refilled by one replica at a time: unlearned positions first,
  rescanned from the start of the book every time and only exhausted —
  no unlearned row left that could ever be claimed, buffered or not —
  opens the partially-learned sweep, from the cursor in `job:cursor`,
  which wraps when that segment runs out; both scans skip what is
  already buffered, since neither ordering knows it queued a position
  before), and the
  `book_stats` hash (serves `GET /api/stats`
  and derives the job floor; updated incrementally on every save, with a
  slow full resync from the DB to correct drift). Everything in Redis is
  derived or ephemeral: `POST /api/redis/rebuild` (worker token) flushes
  it all and rebuilds `book_stats`, for a rollout that changes value
  encodings
- edax — external binary; `EDAX_PATH` in `.env` (see `.env.sample`)
- Frontend — Go `html/template` + vanilla JS/CSS in `static/`, no build step
- Scripts: `local.sh` (dev stack: compose Postgres/Redis, migrate, seed,
  server), `test.sh` (full test suite), `archive_book.sh` (gzipped pg_dump
  of the book to `backups/`), `sandbox.sh` (Claude in an sbx sandbox)
- CI: `.github/workflows/ci.yml` (fmt, golangci-lint, tests);
  `publish.yml` pushes multi-arch (amd64/arm64) server, worker, and
  migrations images to ghcr.io on pushes to `main` and tags, tagged by
  commit sha — deployments pin a sha, there is no `latest`
