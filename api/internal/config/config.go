package config

import (
	"log"
	"os"
)

// Config holds all configuration values loaded from environment variables
type Config struct {
	ServerHost        string
	ServerPort        string
	RedisURL          string
	PostgresURL       string
	BasicAuthUsername string
	BasicAuthPassword string
	Token             string
	Prefork           bool
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		ServerHost:        getEnvMust("FLIPPY_BOOK_SERVER_HOST"),
		ServerPort:        getEnvMust("FLIPPY_BOOK_SERVER_PORT"),
		RedisURL:          getEnvMust("FLIPPY_REDIS_URL"),
		PostgresURL:       getEnvMust("FLIPPY_POSTGRES_URL"),
		BasicAuthUsername: getEnvMust("FLIPPY_BOOK_SERVER_BASIC_AUTH_USER"),
		BasicAuthPassword: getEnvMust("FLIPPY_BOOK_SERVER_BASIC_AUTH_PASS"),
		Token:             getEnvMust("FLIPPY_BOOK_SERVER_TOKEN"),
		Prefork:           getEnvMustBool("FLIPPY_BOOK_SERVER_PREFORK"),
	}
}

// getEnvMust either returns the environment variable or logs a fatal error if it is not set
func getEnvMust(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Environment variable %s is not set", key)
	}
	return value
}

func getEnvMustBool(key string) bool {
	value := getEnvMust(key)

	if value != "true" && value != "false" {
		log.Fatalf("Environment variable %s must be \"true\" or \"false\"", key)
	}

	return value == "true"
}

// StaticDir is the path to the static directory
var StaticDir = "../src/flippy/book/static"
