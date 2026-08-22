package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lk16/flippy/internal/version"
)

// readyCheckTTL is how long a dependency ping outcome (good or bad) is reused before re-checking,
// so frequent probes don't hammer Postgres and Redis.
const readyCheckTTL = 5 * time.Second

// handleHealthz handles GET /healthz: 200 as soon as the listener is up, with no dependency checks.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleReadyz handles GET /readyz: 200 only when this replica can serve correct book data — the
// minimax cache has been built at least once and Postgres and Redis respond to pings.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.cache.Built() {
		writeError(w, http.StatusServiceUnavailable, errors.New("minimax cache not built yet"))
		return
	}

	if err := s.pingDependencies(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// pingDependencies pings Postgres and Redis, caching the outcome for readyCheckTTL.
func (s *Server) pingDependencies(ctx context.Context) error {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()

	if time.Since(s.readyCheckedAt) < readyCheckTTL {
		return s.readyErr
	}

	err := s.repo.Ping(ctx)
	if err == nil {
		if redisErr := s.redis.Ping(ctx).Err(); redisErr != nil {
			err = fmt.Errorf("failed to ping redis: %w", redisErr)
		}
	}

	s.readyCheckedAt = time.Now()
	s.readyErr = err
	return err
}

// handleVersion handles GET /version: reports the git commit this binary was built from.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, versionResponse{Commit: version.Get()})
}

// versionResponse is the JSON body returned by GET /version.
type versionResponse struct {
	Commit string `json:"commit"`
}
