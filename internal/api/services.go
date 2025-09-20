package api

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Load the postgres driver
	"github.com/lk16/flippy/api/internal/config"
	"github.com/redis/go-redis/v9"
)

// Services contains the connections to the external services.
type Services struct {
	Postgres *sqlx.DB
	Redis    *redis.Client
}

func InitServices(cfg *config.ServerConfig) (*Services, error) {
	// Initialize database
	postgres, err := InitPostgres(cfg.PostgresURL)
	if err != nil {
		return nil, err
	}

	// Initialize Redis
	redis, err := InitRedis(cfg.RedisURL)
	if err != nil {
		return nil, err
	}

	return &Services{
		Postgres: postgres,
		Redis:    redis,
	}, nil
}

// InitRedis initializes the Redis connection.
func InitRedis(url string) (*redis.Client, error) {
	// Parse Redis URL
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("error parsing Redis URL: %w", err)
	}

	// Create Redis client
	client := redis.NewClient(opts)

	// Test the connection
	if err = client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("error pinging Redis: %w", err)
	}

	return client, nil
}

// InitPostgres initializes the database connection.
func InitPostgres(url string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	return db, nil
}
