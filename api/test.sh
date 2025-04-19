#!/bin/bash

set -e

COMPOSE_FILE="docker-compose-test.yml"

# Function to clean up containers
cleanup() {
    echo "Cleaning up containers..."
    docker compose -f $COMPOSE_FILE down
}

# Set up trap to run cleanup on script exit
trap cleanup EXIT

# Start supportingcontainers
echo "Starting test containers..."
docker compose -f $COMPOSE_FILE up -d test-redis test-postgres

echo "Waiting for test-postgres to be healthy..."
while ! docker compose -f $COMPOSE_FILE ps test-postgres | grep -q "healthy"; do
    sleep 1
done

# Initialize database
docker compose -f $COMPOSE_FILE exec -T test-postgres psql -U pg-test-user -d pg-test-db < ../schema.sql

# Start application
docker compose -f $COMPOSE_FILE build test-app
docker compose -f $COMPOSE_FILE up -d test-app

# Run tests
echo "Running tests..."
go test -v ./internal/tests/...
