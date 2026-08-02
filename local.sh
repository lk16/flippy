#!/usr/bin/env bash
# Runs the local dev stack: brings up docker-compose.yml (Postgres + Redis,
# with Postgres persisted to a named Docker volume across runs), waits for
# it to be healthy, applies migrations, seeds the precomputed 12-disc board
# set, then runs the webserver in the foreground. Everything started here is
# torn down when the script exits or is killed — except the volume itself,
# which `docker compose down` (no `--volumes`) leaves intact.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

# Load local dev config (e.g. EDAX_PATH) so anything relying on it works.
# The exports below still take precedence, keeping DB/Redis pointed at the
# containers this script itself brings up.
if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi

COMPOSE_FILE="docker-compose.yml"
export FLIPPY_POSTGRES_URL="postgres://flippy:flippy@localhost:12321/flippy?sslmode=disable"
export FLIPPY_REDIS_URL="redis://localhost:12323/0"

# A real binary, not `go run`, so that killing SERVER_PID below reliably
# reaches the actual server process and its own graceful-shutdown handling
# — go run's wrapper process doesn't always forward signals to the binary
# it execs when killed by PID rather than via a terminal's Ctrl+C.
BIN="$(mktemp)"
SERVER_PID=""

cleanup() {
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    docker compose -f "$COMPOSE_FILE" down
    rm -f "$BIN"
}
trap cleanup EXIT INT TERM

# Named explicitly: `--wait` with no service names also waits on `worker`, which is
# scaled to 0 replicas by default, and docker compose mishandles that as a missing
# dependency rather than an intentionally-absent service.
docker compose -f "$COMPOSE_FILE" up -d --wait postgres redis

migrate -path migrations -database "$FLIPPY_POSTGRES_URL" up

# Idempotent: only adds boards missing from the DB, never touches existing
# rows (see internal/loader.SeedBoards).
go run ./cmd/loader seed

go build -o "$BIN" ./cmd/server

"$BIN" &
SERVER_PID=$!
wait "$SERVER_PID"
