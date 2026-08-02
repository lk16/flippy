# Linting and testing

## Lint / format

`pre-commit run -a` — gofmt, `golangci-lint run --fix ./cmd/... ./internal/...`,
plus whitespace/EOL/YAML hooks (`.pre-commit-config.yaml`). CI runs the
same checks.

## Tests

`./test.sh` runs everything:

1. JS unit tests: `node static/test/run.js` (no deps, tiny custom framework)
2. Ephemeral Postgres + Redis via `docker-compose.test.yml`, migrations applied
3. `gotestsum -- -p 1 -cover ./...` plus a per-function coverage report

Notes:

- `-p 1` is required: packages share the test DB and Postgres deadlocks
  under default parallelism.
- Coverage doesn't prove correctness, but uncovered lines outside
  `cmd/*/main.go` flag missing tests.
- `internal/edax` subprocess tests need a real edax binary (`EDAX_PATH` in
  `.env`) and skip themselves without one (as in CI); parser and
  problem-format tests always run.
