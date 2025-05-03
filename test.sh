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

# Initialize database schema
docker compose -f $COMPOSE_FILE exec -T test-postgres psql -U pg-test-user -d pg-test-db < ./schema.sql

# Load test data
docker compose -f $COMPOSE_FILE exec -T test-postgres psql -U pg-test-user -d pg-test-db < ./test_data.sql

# Start application
docker compose -f $COMPOSE_FILE up -d --build test-app

# Wait for server to be ready
echo "Waiting for server to be ready..."
while ! curl -s http://localhost:3000/version > /dev/null; do
    sleep 1
done
echo "Server is ready"

# Run tests
echo "Running tests..."
export FLIPPY_REDIS_URL='redis://localhost:6380'
export FLIPPY_POSTGRES_URL='postgres://pg-test-user:pg-test-password@localhost:5433/pg-test-db?sslmode=disable'
export FLIPPY_BOOK_SERVER_HOST='localhost'
export FLIPPY_BOOK_SERVER_PORT='3000'
export FLIPPY_BOOK_SERVER_BASIC_AUTH_USER='test-user'
export FLIPPY_BOOK_SERVER_BASIC_AUTH_PASS='test-password'
export FLIPPY_BOOK_SERVER_TOKEN='test-token'
export FLIPPY_BOOK_SERVER_PREFORK='false'

go test -v ./internal/...
