package config

import (
	"log/slog"
	"os"
)

const (
	MinBookLearnLevel   = 16
	MaxBookSavableDiscs = 30
)

// ServerConfig holds all configuration values loaded from environment variables.
type ServerConfig struct {
	ServerHost        string
	ServerPort        string
	RedisURL          string
	PostgresURL       string
	BasicAuthUsername string
	BasicAuthPassword string
	Token             string
	Prefork           bool
	StaticDir         string
}

// LoadServerConfig loads configuration from environment variables.
func LoadServerConfig() *ServerConfig {
	return &ServerConfig{
		ServerHost:        getEnvMust("FLIPPY_BOOK_SERVER_HOST"),
		ServerPort:        getEnvMust("FLIPPY_BOOK_SERVER_PORT"),
		RedisURL:          getEnvMust("FLIPPY_REDIS_URL"),
		PostgresURL:       getEnvMust("FLIPPY_POSTGRES_URL"),
		BasicAuthUsername: getEnvMust("FLIPPY_BOOK_SERVER_BASIC_AUTH_USER"),
		BasicAuthPassword: getEnvMust("FLIPPY_BOOK_SERVER_BASIC_AUTH_PASS"),
		Token:             getEnvMust("FLIPPY_BOOK_SERVER_TOKEN"),
		Prefork:           getEnvMustBool("FLIPPY_BOOK_SERVER_PREFORK"),
		StaticDir:         getEnvMust("FLIPPY_BOOK_SERVER_STATIC_DIR"),
	}
}

type LearnClientConfig struct {
	ServerURL string
	Token     string
}

func LoadLearnClientConfig() *LearnClientConfig {
	return &LearnClientConfig{
		ServerURL: getEnvMust("FLIPPY_BOOK_SERVER_URL"),
		Token:     getEnvMust("FLIPPY_BOOK_SERVER_TOKEN"),
	}
}

type EdaxConfig struct {
	EdaxPath string
}

func LoadEdaxConfig() *EdaxConfig {
	return &EdaxConfig{
		EdaxPath: getEnvMust("FLIPPY_EDAX_PATH"),
	}
}

// getEnvMust either returns the environment variable or logs a fatal error if it is not set.
func getEnvMust(key string) string {
	value := os.Getenv(key)
	if value == "" {
		slog.Error("Environment variable is not set", "key", key)
		os.Exit(1)
	}
	return value
}

func getEnvMustBool(key string) bool {
	value := getEnvMust(key)

	if value != "true" && value != "false" {
		slog.Error("Cannot load environment variable, it must be \"true\" or \"false\"", "key", key, "value", value)
		os.Exit(1)
	}

	return value == "true"
}
