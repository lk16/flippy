# Project

Flippy builds an Othello opening book: positions are evaluated by edax,
stored in Postgres, and browsed via a web frontend.

## Components

- `cmd/server` (`internal/api`, `internal/web`) — REST API + websocket +
  HTML admin pages (game/analysis, stats, clients). Serves evaluations from
  the DB, with an in-memory minimax cache (`internal/book`) that backfills
  every <12-disc position from the 12-disc evaluations.
- `cmd/worker` (`internal/worker`, `internal/edax`) — claims jobs from the
  server, evaluates them with one long-running edax subprocess, submits
  results, heartbeats. Job claims and worker identity live only in Redis
  (TTL keys), never in Postgres.
- `cmd/loader` (`internal/loader`) — subcommands `seed` (precomputed
  12-disc set, embedded in source), `load` (PGN files), `load-oq`
  (Othello Quest move strings). Add-only: never updates or removes rows.
- `internal/othello` — bitboard `Board`/`Game`, move generation,
  `NormalizedBoard` (canonical across the 8 symmetries, turn preserved),
  wtb/PGN/Othello Quest parsers. `static/board.js` reimplements the
  bitboard logic in JS (verified byte-for-byte against Go output) so the
  browser can simulate moves locally.

## Stack

- Go (version in `go.mod`); deps: pgx, go-redis, coder/websocket, testify, godotenv
- Postgres — single `boards` table (position, disc count, level, score;
  depth and confidence follow from disc count + level, see
  `internal/edax.SearchParams`); migrations via golang-migrate
  (`migrations/`), one-shot operator SQL in `scripts/`
- Redis — job claims, worker heartbeats, stats cache
- edax — external binary; `EDAX_PATH` in `.env` (see `.env.sample`)
- Frontend — Go `html/template` + vanilla JS/CSS in `static/`, no build step
- Scripts: `local.sh` (dev stack: compose Postgres/Redis, migrate, seed,
  server), `test.sh` (full test suite), `archive_book.sh` (gzipped pg_dump
  of the book to `backups/`), `sandbox.sh` (Claude in an sbx sandbox)
- CI: `.github/workflows/ci.yml` (fmt, golangci-lint, tests)
