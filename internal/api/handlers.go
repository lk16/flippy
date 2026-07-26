package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// writeJSON encodes v as the JSON response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes err's message as a JSON error response.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// handleGetJob handles GET /api/jobs: atomically claims and returns the
// next available job for the requesting worker, or 204 No Content if none
// is available right now.
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	workerID := r.URL.Query().Get("worker_id")
	if workerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing worker_id"))
		return
	}

	job, ok, err := s.claimJob(r.Context(), workerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, jobResponse{Board: job.Board.String(), Level: job.Level})
}

// validateJobResult checks the bounds of an evaluation submitted for a job
// result. It doesn't duplicate storage-layer concerns (e.g. whether it's an
// improvement over what's stored) — only whether the values are sane.
func validateJobResult(req jobResultRequest) error {
	if req.Level <= 0 {
		return errors.New("level must be positive")
	}
	if req.Depth < 0 || req.Depth > 60 {
		return errors.New("depth out of range")
	}
	if req.Score < -64 || req.Score > 64 {
		return errors.New("score out of range")
	}
	return nil
}

// handleSubmitJobResult handles POST /api/jobs/result: stores a worker's
// evaluation for a board and releases its job claim.
func (s *Server) handleSubmitJobResult(w http.ResponseWriter, r *http.Request) {
	var req jobResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing worker_id"))
		return
	}

	board, err := othello.ParseBoard(req.Board)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid board: %w", err))
		return
	}

	normalized, err := othello.NewNormalizedBoard(board)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("board must be normalized: %w", err))
		return
	}

	if err := validateJobResult(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	eval := db.Evaluation{Level: req.Level, Depth: req.Depth, Confidence: req.Confidence, Score: req.Score}
	if err := s.repo.SaveEvaluation(r.Context(), normalized, eval); err != nil {
		if errors.Is(err, db.ErrBoardNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if err := s.releaseClaim(r.Context(), normalized.String(), req.WorkerID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleGetBoard handles GET /api/boards?board=<board string>: looks up the
// stored evaluation for a (not necessarily normalized) board.
func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	boardParam := r.URL.Query().Get("board")
	if boardParam == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing board"))
		return
	}

	board, err := othello.ParseBoard(boardParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid board: %w", err))
		return
	}

	eval, err := s.repo.GetBoard(r.Context(), board)
	if err != nil {
		if errors.Is(err, db.ErrBoardNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, evaluationResponse{
		Level:      eval.Level,
		Depth:      eval.Depth,
		Confidence: eval.Confidence,
		Score:      eval.Score,
	})
}

// handleHeartbeat handles POST /api/workers/heartbeat: refreshes the TTL of
// the requesting worker's job claim, if it has one, keeping it from being
// reaped and reassigned to another worker.
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing worker_id"))
		return
	}

	if err := s.heartbeat(r.Context(), req.WorkerID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleStats handles GET /api/stats: returns move-counts per (disc count,
// level) cell.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	entries := make([]statEntry, len(stats))
	for i, stat := range stats {
		entries[i] = statEntry{DiscCount: stat.DiscCount, Level: stat.Level, Count: stat.Count}
	}

	writeJSON(w, http.StatusOK, entries)
}
