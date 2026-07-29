package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// minJobsPerRequest and maxJobsPerRequest bound the count param on GET /api/jobs.
const (
	minJobsPerRequest = 1
	maxJobsPerRequest = 10
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

// handleGetJob handles GET /api/jobs: claims and returns up to count available jobs as a JSON array,
// or 204 if none.
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	workerID := r.URL.Query().Get("worker_id")
	if workerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing worker_id"))
		return
	}

	countParam := r.URL.Query().Get("count")
	if countParam == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing count"))
		return
	}
	count, err := strconv.Atoi(countParam)
	if err != nil || count < minJobsPerRequest || count > maxJobsPerRequest {
		writeError(w, http.StatusBadRequest,
			fmt.Errorf("count must be an integer between %d and %d", minJobsPerRequest, maxJobsPerRequest))
		return
	}

	jobs, err := s.claimJobs(r.Context(), workerID, count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(jobs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	responses := make([]jobResponse, len(jobs))
	for i, job := range jobs {
		responses[i] = jobResponse{Board: job.Board.String(), Level: job.Level}
	}

	writeJSON(w, http.StatusOK, responses)
}

// validateJobResult checks that a submitted evaluation's values are within sane bounds.
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

// handleSubmitJobResult handles POST /api/jobs/result: stores a worker's evaluation and releases its claim.
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

	// Only a leaf-disc-count save can change the minimax backfill.
	if normalized.CountDiscs() == book.LeafDiscs {
		if err := s.cache.Rebuild(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	if err := s.recordJobCompletion(r.Context(), req.WorkerID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if err := s.releaseClaim(r.Context(), normalized.String(), req.WorkerID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// lookupEvaluation returns board's evaluation from the DB, falling back to the minimax cache; ok is
// false if neither has a real (learned) result.
func (s *Server) lookupEvaluation(ctx context.Context, board othello.Board) (evaluationResponse, bool, error) {
	if !board.HasMoves() {
		return s.lookupPassEvaluation(ctx, board)
	}

	eval, err := s.repo.GetBoard(ctx, board)
	if err == nil && eval.IsLearned() {
		return evaluationResponse{
			Level:      eval.Level,
			Depth:      eval.Depth,
			Confidence: eval.Confidence,
			Score:      eval.Score,
			Source:     evaluationSourceEdax,
		}, true, nil
	}
	if err != nil && !errors.Is(err, db.ErrBoardNotFound) {
		return evaluationResponse{}, false, err
	}

	if score, ok := s.cache.Get(board); ok {
		return evaluationResponse{Score: score, Source: evaluationSourceMinimax}, true, nil
	}

	return evaluationResponse{}, false, nil
}

// lookupPassEvaluation handles a forced-pass board: negates the other player's evaluation, or returns
// the final score if the pass ends the game.
func (s *Server) lookupPassEvaluation(ctx context.Context, board othello.Board) (evaluationResponse, bool, error) {
	passed, err := board.DoMove(othello.PassMove)
	if err != nil {
		return evaluationResponse{}, false, err
	}

	if !passed.HasMoves() {
		return evaluationResponse{Score: board.FinalScore(), Source: evaluationSourceFinal}, true, nil
	}

	eval, ok, err := s.lookupEvaluation(ctx, passed)
	if err != nil || !ok {
		return evaluationResponse{}, ok, err
	}

	eval.Score = -eval.Score
	return eval, true, nil
}

// handleGetBoard handles GET /api/boards?board=<board string>: looks up a board's evaluation.
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

	eval, ok, err := s.lookupEvaluation(r.Context(), board)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, db.ErrBoardNotFound)
		return
	}

	writeJSON(w, http.StatusOK, eval)
}

// handleHeartbeat handles POST /api/workers/heartbeat: marks the worker active and refreshes its claim TTL.
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

	if err := s.heartbeat(r.Context(), req.WorkerID, req.Hostname, req.GitCommit); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleListWorkers handles GET /api/workers: returns active workers, ordered by positions computed.
func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := s.listWorkers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	entries := make([]workerResponse, len(workers))
	for i, wk := range workers {
		entries[i] = workerResponse(wk)
	}

	writeJSON(w, http.StatusOK, entries)
}

// handleStats handles GET /api/stats: returns move-counts per (disc count, level) cell.
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
