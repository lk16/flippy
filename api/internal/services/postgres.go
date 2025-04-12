package services

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// InitPostgres initializes the database connection
func InitPostgres() (*sqlx.DB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("FLIPPY_POSTGRES_USER"),
		os.Getenv("FLIPPY_POSTGRES_PASS"),
		os.Getenv("FLIPPY_POSTGRES_HOST"),
		os.Getenv("FLIPPY_POSTGRES_PORT"),
		os.Getenv("FLIPPY_POSTGRES_DB"),
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %w", err)
	}

	return db, nil
}
