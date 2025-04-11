package db

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

// InitRedis initializes the Redis connection
func InitRedis() error {
	// Get Redis URL from environment variable
	redisURL := os.Getenv("FLIPPY_REDIS_URL")
	if redisURL == "" {
		return fmt.Errorf("FLIPPY_REDIS_URL is not set")
	}

	// Parse Redis URL
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("error parsing Redis URL: %w", err)
	}

	// Create Redis client
	redisClient = redis.NewClient(opts)

	// Test the connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("error pinging Redis: %w", err)
	}

	return nil
}

// GetRedis returns the Redis client
func GetRedis() *redis.Client {
	return redisClient
}
