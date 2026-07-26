#!/usr/bin/env bash
# Dumps the boards table (the opening book) to a timestamped, gzipped SQL
# file in backups/, for backup/archival purposes. Runs pg_dump inside the
# running postgres container rather than on the host, since it must match
# the server's own version.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi

if [ -z "${FLIPPY_POSTGRES_URL:-}" ]; then
    echo "FLIPPY_POSTGRES_URL is not set (add it to .env)" >&2
    exit 1
fi

# Pull user/password/dbname out of FLIPPY_POSTGRES_URL, since pg_dump runs
# inside the postgres container below and must reach it at localhost:5432
# (its own internal port) rather than the host-mapped port the URL points
# at from outside.
if [[ ! "$FLIPPY_POSTGRES_URL" =~ ^postgres://([^:]+):([^@]+)@[^/]+/([^?]+) ]]; then
    echo "failed to parse FLIPPY_POSTGRES_URL" >&2
    exit 1
fi
PG_USER="${BASH_REMATCH[1]}"
PG_PASSWORD="${BASH_REMATCH[2]}"
PG_DATABASE="${BASH_REMATCH[3]}"

mkdir -p backups
SQL_FILE="backups/boards_$(date +%s).sql"

docker compose -f docker-compose.yml exec -T postgres \
    pg_dump "postgres://${PG_USER}:${PG_PASSWORD}@localhost:5432/${PG_DATABASE}" \
    -t boards --data-only > "$SQL_FILE"
gzip "$SQL_FILE"

echo "wrote ${SQL_FILE}.gz"
