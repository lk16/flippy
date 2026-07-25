#!/usr/bin/env bash
# Runs the full test suite against an ephemeral, isolated Postgres instance:
# brings up docker-compose.test.yml, waits for it to be healthy, applies
# migrations, runs tests, then tears the test infrastructure back down.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

COMPOSE_FILE="docker-compose.test.yml"
export FLIPPY_POSTGRES_URL="postgres://flippy_test:flippy_test@localhost:12322/flippy_test?sslmode=disable"
COVERPROFILE="$(mktemp)"

cleanup() {
    docker compose -f "$COMPOSE_FILE" down --volumes
    rm -f "$COVERPROFILE"
}
trap cleanup EXIT

docker compose -f "$COMPOSE_FILE" up -d --wait

migrate -path migrations -database "$FLIPPY_POSTGRES_URL" up

gotestsum -- -cover -coverprofile="$COVERPROFILE" ./...

echo
go tool cover -func="$COVERPROFILE"
