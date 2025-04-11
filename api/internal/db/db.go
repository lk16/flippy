package db

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var db *sqlx.DB

// InitDB initializes the database connection
func InitDB() error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("FLIPPY_POSTGRES_USER"),
		os.Getenv("FLIPPY_POSTGRES_PASS"),
		os.Getenv("FLIPPY_POSTGRES_HOST"),
		os.Getenv("FLIPPY_POSTGRES_PORT"),
		os.Getenv("FLIPPY_POSTGRES_DB"),
	)

	fmt.Println("Connecting to database with DSN:", dsn)

	var err error
	db, err = sqlx.Connect("postgres", dsn)
	if err != nil {
		return fmt.Errorf("error connecting to database: %w", err)
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		return fmt.Errorf("error pinging database: %w", err)
	}

	return nil
}

// GetDB returns the database connection
func GetDB() *sqlx.DB {
	return db
}
