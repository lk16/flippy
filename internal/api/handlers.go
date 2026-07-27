package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/lk16/flippy/internal/book"
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

	// The minimax backfill only depends on leaf-disc-count evaluations, so
	// only a save at that disc count can change it.
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

// lookupEvaluation returns the evaluation for board: a direct DB result if
// one exists and has actually been learned, otherwise a minimax-derived one
// from the cache. ok is false if neither has it — including when board has
// a row but it's still at its zero-valued, not-yet-learned state, so
// callers don't render that as a real (and misleadingly draw-like) score.
//
// If the player to move has no legal move, board itself is never stored
// (see loader.ExtractBoards/isSavable): only positions with a legal move
// are. Its evaluation is defined instead as the negation of the evaluation
// after the forced pass — the same position, just with the other player to
// move — recursing at most once, since two passes in a row end the game.
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

// lookupPassEvaluation handles a board where the player to move has no
// legal move. If the forced pass leaves the other player able to move, its
// evaluation is looked up and negated back to board's perspective. If the
// pass doesn't leave a legal move either, the game is over: board's actual
// final score is returned rather than an edax/minimax estimate.
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

// handleHeartbeat handles POST /api/workers/heartbeat: records the
// requesting worker as active, and refreshes the TTL of its job claim, if
// it has one, keeping it from being reaped and reassigned to another
// worker.
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

// handleListWorkers handles GET /api/workers: returns every currently
// active worker, ordered by positions computed (most first).
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
