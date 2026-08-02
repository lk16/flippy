// Package env loads local development configuration from a .env file
// before a binary reads its environment variables.
package env

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Load reads .env into the process environment without overriding variables already set; a missing
// .env file is not an error.
func Load() error {
	if err := godotenv.Load(); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load .env: %w", err)
	}
	return nil
}
