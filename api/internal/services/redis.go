package services

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

// Connect initializes the Redis connection
func InitRedis() (*redis.Client, error) {
	// Get Redis URL from environment variable
	redisURL := os.Getenv("FLIPPY_REDIS_URL")
	if redisURL == "" {
		return nil, fmt.Errorf("FLIPPY_REDIS_URL is not set")
	}

	// Parse Redis URL
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing Redis URL: %w", err)
	}

	// Create Redis client
	client := redis.NewClient(opts)

	// Test the connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("error pinging Redis: %w", err)
	}

	return client, nil
}
