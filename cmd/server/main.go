package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lk16/flippy/internal/api"
	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/env"
	"github.com/lk16/flippy/internal/web"
)

const shutdownTimeout = 10 * time.Second

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s environment variable is not set", name)
	}
	return value
}

func main() {
	if err := env.Load(); err != nil {
		log.Fatalf("failed to load .env: %v", err)
	}

	postgresURL := requiredEnv("FLIPPY_POSTGRES_URL")
	redisURL := requiredEnv("FLIPPY_REDIS_URL")

	addr := os.Getenv("FLIPPY_SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, postgresURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	redisClient, err := api.NewRedisClient(ctx, redisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer func() { _ = redisClient.Close() }()

	repo := db.NewRepository(pool)

	cache := book.NewCache(repo)
	if err := cache.Rebuild(ctx); err != nil {
		log.Fatalf("failed to build minimax cache: %v", err)
	}
	log.Printf("minimax cache built: %d boards", cache.Len())

	apiServer := api.NewServer(repo, redisClient, cache)

	webServer, err := web.NewServer()
	if err != nil {
		log.Fatalf("failed to build web server: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/ws", apiServer.Handler())
	mux.Handle("/", webServer.Handler())

	httpServer := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("error shutting down server: %v", err)
		}
	}()

	log.Printf("flippy server listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
