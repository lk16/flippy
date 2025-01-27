#!/bin/bash
TIMESTAMP=$(date +%s)
SQL_FILE="ignored/edax_${TIMESTAMP}.sql"

docker exec flippy-postgres-1 pg_dump 'postgres://book_user:book_pass@localhost:5432/flippy' -t edax --data-only > "${SQL_FILE}"
gzip "${SQL_FILE}"
