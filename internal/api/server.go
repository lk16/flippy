// Package api implements flippy's JSON REST API: job assignment for
// workers, evaluation lookup, and book statistics.
package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
)

// Server holds the dependencies shared by every API handler.
type Server struct {
	repo        *db.Repository
	redis       *redis.Client
	cache       *book.Cache
	workerToken string

	// replicaID names this replica in the stats-refresh lock, purely for debugging; under
	// Kubernetes the hostname is the pod name.
	replicaID string

	// Cached dependency-ping outcome for /readyz (see pingDependencies).
	readyMu        sync.Mutex
	readyCheckedAt time.Time
	readyErr       error
}

// NewServer returns a Server backed by repo, redisClient, and cache. workerToken guards the
// endpoints that mutate the book (see requireWorkerToken).
func NewServer(repo *db.Repository, redisClient *redis.Client, cache *book.Cache, workerToken string) *Server {
	replicaID, err := os.Hostname()
	if err != nil {
		replicaID = "unknown"
	}

	return &Server{repo: repo, redis: redisClient, cache: cache, workerToken: workerToken, replicaID: replicaID}
}

// Handler returns the HTTP handler serving the JSON REST API and the "/ws" websocket endpoint.
// Endpoints workers use to fetch and mutate book state require the worker token; read-only
// browsing endpoints and /api/pgn (a pure parser, it writes nothing) stay open for the web UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/jobs", s.requireWorkerToken(s.handleGetJob))
	mux.HandleFunc("POST /api/jobs/result", s.requireWorkerToken(s.handleSubmitJobResult))
	mux.HandleFunc("POST /api/jobs/release", s.requireWorkerToken(s.handleReleaseJob))
	mux.HandleFunc("GET /api/boards", s.handleGetBoard)
	mux.HandleFunc("POST /api/workers/heartbeat", s.requireWorkerToken(s.handleHeartbeat))
	mux.HandleFunc("GET /api/workers", s.handleListWorkers)
	mux.HandleFunc("POST /api/redis/rebuild", s.requireWorkerToken(s.handleRebuildRedis))
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/level-config", s.handleLevelConfig)
	mux.HandleFunc("POST /api/pgn", s.handlePGN)
	mux.HandleFunc("GET /ws", s.handleWebSocket)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /version", s.handleVersion)

	return mux
}

// requireWorkerToken rejects requests whose Authorization header does not carry the configured
// worker token as a bearer token, so exposing the server beyond localhost doesn't let anyone
// poison the book.
func (s *Server) requireWorkerToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !workerTokenMatches(s.workerToken, token) {
			writeError(w, http.StatusUnauthorized, errors.New("missing or invalid worker token"))
			return
		}
		next(w, r)
	}
}

// workerTokenMatches compares a presented token against the configured one in constant time;
// hashing first also hides length information. An empty configured token matches nothing, so a
// server misconfigured without one fails closed.
func workerTokenMatches(configured, presented string) bool {
	if configured == "" {
		return false
	}
	configuredSum := sha256.Sum256([]byte(configured))
	presentedSum := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(configuredSum[:], presentedSum[:]) == 1
}
