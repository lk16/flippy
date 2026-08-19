package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
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

// statusRecorder wraps a http.ResponseWriter to capture the status code written, since
// http.ResponseWriter doesn't expose it directly.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Hijack lets the wrapped writer be used for connection upgrades (the "/ws"
// WebSocket endpoint). Without it, embedding the http.ResponseWriter interface
// hides the underlying Hijacker, so websocket.Accept fails with a 501.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}

// Unwrap exposes the underlying writer to http.ResponseController (Flush, etc.).
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// logRequests wraps next, logging method, path, status code, and duration for every request.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
		)
	})
}

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
		addr = ":7777"
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
	go apiServer.RunBookStatsRefresh(ctx)

	webServer, err := web.NewServer()
	if err != nil {
		log.Fatalf("failed to build web server: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/ws", apiServer.Handler())
	mux.Handle("/", webServer.Handler())

	httpServer := &http.Server{Addr: addr, Handler: logRequests(mux)}

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
