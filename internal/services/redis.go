package services

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Connect initializes the Redis connection
func InitRedis(url string) (*redis.Client, error) {
	// Parse Redis URL
	opts, err := redis.ParseURL(url)
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
