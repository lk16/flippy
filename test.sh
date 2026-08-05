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

# Rust unit tests (wasm/edax-eval/). Also need no infrastructure, so run them
# here too, before the Docker compose stack comes up.
echo "Running Rust checks…"
cargo fmt --manifest-path wasm/edax-eval/Cargo.toml -- --check
cargo clippy --manifest-path wasm/edax-eval/Cargo.toml -- -Dwarnings
# --release: the differential test (tests/differential.rs, EDAX_PATH-gated) runs an unoptimized
# depth-10 search in minutes rather than seconds without it; skipped entirely when EDAX_PATH isn't
# set (e.g. in CI), so this only matters for local runs with the real edax binary configured.
cargo test --manifest-path wasm/edax-eval/Cargo.toml --release

# wasm/edax-eval/js/edax-eval.test.js (Task 10) exercises the real compiled .wasm module, so it
# needs one built first. --lib: the extract_weights bin tool was never meant to target wasm32 (see
# Cargo.toml).
cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown --lib --release
node wasm/edax-eval/js/edax-eval.test.js
# Checks the checked-in dist/edax_eval.wasm (what the browser actually gets) against that fresh
# build -- nothing else in this suite would notice dist/ going stale.
node wasm/edax-eval/js/dist-freshness.test.js

docker compose -f "$COMPOSE_FILE" up -d --wait

migrate -path migrations -database "$FLIPPY_POSTGRES_URL" up

gotestsum -- -p 1 -cover -coverprofile="$COVERPROFILE" ./...

echo
go tool cover -func="$COVERPROFILE"
