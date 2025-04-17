package services

import (
	"github.com/jmoiron/sqlx"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/redis/go-redis/v9"
)

// Services contains the connections to the external services
type Services struct {
	Postgres *sqlx.DB
	Redis    *redis.Client
}

// GetServices initializes the services
func GetServices(cfg *config.Config) (*Services, error) {
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
