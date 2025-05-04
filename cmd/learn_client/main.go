package main

import (
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
)

func setLogLevel() {
	level := slog.LevelInfo
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		switch strings.ToUpper(envLevel) {
		case "DEBUG":
			level = slog.LevelDebug
		case "INFO":
			level = slog.LevelInfo
		case "WARN":
			level = slog.LevelWarn
		case "ERROR":
			level = slog.LevelError
		default:
			slog.Error("Invalid log level", "level", envLevel)
			os.Exit(1)
		}
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

func main() {
	setLogLevel()

	cfg := config.LoadLearnClientConfig()

	learnClient, err := book.NewLearnClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create learn client: %v", err)
	}

	learnClient.Run()
}
