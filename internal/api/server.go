// Package api implements flippy's JSON REST API: job assignment for
// workers, evaluation lookup, and book statistics.
package api

import (
	"net/http"

	"github.com/redis/go-redis/v9"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
)

// Server holds the dependencies shared by every API handler.
type Server struct {
	repo  *db.Repository
	redis *redis.Client
	cache *book.Cache
}

// NewServer returns a Server backed by repo, redisClient, and cache.
func NewServer(repo *db.Repository, redisClient *redis.Client, cache *book.Cache) *Server {
	return &Server{repo: repo, redis: redisClient, cache: cache}
}

// Handler returns the HTTP handler serving the JSON REST API and the "/ws" websocket endpoint.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/jobs", s.handleGetJob)
	mux.HandleFunc("POST /api/jobs/result", s.handleSubmitJobResult)
	mux.HandleFunc("GET /api/boards", s.handleGetBoard)
	mux.HandleFunc("POST /api/workers/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /api/workers", s.handleListWorkers)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/level-config", s.handleLevelConfig)
	mux.HandleFunc("POST /api/pgn", s.handlePGN)
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	return mux
}
