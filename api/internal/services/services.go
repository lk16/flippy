package services

import (
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// Services contains the connections to the external services
type Services struct {
	Postgres *sqlx.DB
	Redis    *redis.Client
}
