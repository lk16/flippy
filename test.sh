#!/usr/bin/env bash
# Runs the full test suite against an ephemeral, isolated Postgres instance:
# brings up docker-compose.test.yml, waits for it to be healthy, applies
# migrations, runs tests, then tears the test infrastructure back down.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

# Load local dev config (e.g. EDAX_PATH) so tests that need it aren't
# skipped. The exports below still take precedence over any values .env
# happens to set, keeping tests isolated from the local dev DB/Redis.
if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi

COMPOSE_FILE="docker-compose.test.yml"
export FLIPPY_POSTGRES_URL="postgres://flippy_test:flippy_test@localhost:12322/flippy_test?sslmode=disable"
export FLIPPY_REDIS_URL="redis://localhost:12324/0"
COVERPROFILE="$(mktemp)"

cleanup() {
    docker compose -f "$COMPOSE_FILE" down --volumes
    rm -f "$COVERPROFILE"
}
trap cleanup EXIT

# Browser JS unit tests (static/). These need no infrastructure, so run them first
# and fail fast (set -e aborts on a non-zero exit) before spinning up containers.
echo "Running JS tests…"
node static/test/run.js

docker compose -f "$COMPOSE_FILE" up -d --wait

migrate -path migrations -database "$FLIPPY_POSTGRES_URL" up

gotestsum -- -p 1 -cover -coverprofile="$COVERPROFILE" ./...

echo
go tool cover -func="$COVERPROFILE"
