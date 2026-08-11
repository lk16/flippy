package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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

// maxLevel bounds a submitted edax search level. It's well above any level flippy actually requests
// (see TargetLevel) but stays within the smallint column the evaluation is stored in, so an
// out-of-range value is a clean 400 rather than a Postgres error surfaced as a 500.
const maxLevel = 60

// validateJobResult checks that a submitted evaluation's values are within sane bounds.
func validateJobResult(req jobResultRequest) error {
	if req.Level <= 0 || req.Level > maxLevel {
		return fmt.Errorf("level must be between 1 and %d", maxLevel)
	}
	if req.Depth < 0 || req.Depth > 60 {
		return errors.New("depth out of range")
	}
	if req.Confidence < 0 || req.Confidence > 100 {
		return errors.New("confidence out of range")
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

	// Always cache the result ephemerally so lookupEvaluation can find it regardless of DB eligibility.
	s.setAnalysisResult(r.Context(), normalized.String(), evaluationResponse{
		Level: req.Level, Depth: req.Depth, Confidence: req.Confidence, Score: req.Score,
		Source: evaluationSourceEdax,
	})

	// Check whether this job originated from the priority queue.
	isPriority, err := s.consumePriorityClaim(r.Context(), normalized.String())
	if err != nil {
		slog.Warn("failed to check priority claim; treating as non-priority", "error", err)
	}

	savedToDB := false
	discCount := normalized.CountDiscs()

	if isPriority {
		if discCount <= book.MaxSavableDiscs {
			if saveErr := s.repo.SaveEvaluation(r.Context(), normalized, eval); saveErr != nil {
				if errors.Is(saveErr, db.ErrBoardNotFound) {
					// Board has no row yet; add one and retry.
					if addErr := s.repo.AddBoards(r.Context(), []othello.NormalizedBoard{normalized}); addErr != nil {
						slog.Error("failed to add priority board", "error", addErr)
					} else if saveErr2 := s.repo.SaveEvaluation(r.Context(), normalized, eval); saveErr2 != nil {
						slog.Error("failed to save priority evaluation after AddBoards", "error", saveErr2)
					} else {
						savedToDB = true
					}
				} else {
					writeError(w, http.StatusInternalServerError, saveErr)
					return
				}
			} else {
				savedToDB = true
			}
		}
		// discCount > MaxSavableDiscs: ephemeral cache is the only record; skip DB entirely.
	} else {
		// Non-priority path: SaveEvaluation must succeed; ErrBoardNotFound is a real bug here since every
		// ListLearnable-originated board is guaranteed to already have a row.
		if err := s.repo.SaveEvaluation(r.Context(), normalized, eval); err != nil {
			if errors.Is(err, db.ErrBoardNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		savedToDB = true
	}

	if savedToDB {
		s.invalidateStatsCache(r.Context())

		// Only a leaf-disc-count save can change the minimax backfill.
		if discCount == book.LeafDiscs {
			if err := s.cache.Rebuild(r.Context()); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
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

// handleReleaseJob handles POST /api/jobs/release: releases a worker's claim on a board it didn't
// finish (e.g. a graceful shutdown with the job still queued or in flight), so another worker can pick
// it up without waiting out claimTTL.
func (s *Server) handleReleaseJob(w http.ResponseWriter, r *http.Request) {
	var req releaseJobRequest
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

	if err := s.releaseClaim(r.Context(), normalized.String(), req.WorkerID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// lookupEvaluation returns board's evaluation from the DB, minimax cache, or ephemeral analysis
// cache; ok is false if none has a real (learned) result.
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

	// Ephemeral cache: covers priority-computed evaluations for boards with no DB row (>30 discs)
	// or not yet persisted.
	if cached, ok, err := s.getAnalysisResult(ctx, board.Normalize().String()); err == nil && ok {
		return cached, true, nil
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
// The result is Redis-cached because the underlying GROUP BY scans every row in the boards table
// and becomes slow at millions of rows. The cache is invalidated whenever an evaluation is saved
// (see handleSubmitJobResult) and expires after statsTTL as a safety net for loader-added boards.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if cached, err := s.getCachedStats(r.Context()); err == nil && cached != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cached)
		return
	}

	stats, err := s.repo.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	entries := make([]statEntry, len(stats))
	for i, stat := range stats {
		entries[i] = statEntry{DiscCount: stat.DiscCount, Level: stat.Level, Count: stat.Count}
	}

	data, err := json.Marshal(entries)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data = append(data, '\n')

	s.setCachedStats(r.Context(), data)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleLevelConfig handles GET /api/level-config: returns the constants the frontend needs to
// determine how many level-increment rounds to request per board.
func (s *Server) handleLevelConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, levelConfigResponse{
		PriorityLevel:   PriorityLevel,
		MaxSavableDiscs: book.MaxSavableDiscs,
		TargetLevels:    TargetLevelTiers(),
	})
}

// pgnBodyLimit caps the PGN request body at 1 MiB; a typical game is well under 1 KiB.
const pgnBodyLimit = 1 << 20

// handlePGN handles POST /api/pgn: parses PGN text (or a compact OthelloQuest move string) and
// returns the board sequence as strings. Only the first game in a multi-game PGN is used.
func (s *Server) handlePGN(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, pgnBodyLimit))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("failed to read body: %w", err))
		return
	}

	text := strings.TrimSpace(string(body))

	if text == "" {
		writeError(w, http.StatusBadRequest, errors.New("empty request body"))
		return
	}

	var firstGame *othello.Game

	if !strings.Contains(text, "[") {
		// No PGN metadata brackets → treat as OthelloQuest compact format (e.g. "e6f4e3d6").
		compact := strings.Join(strings.Fields(text), "")
		game, parseErr := othello.ParseOthelloQuestMoves(compact)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, parseErr)
			return
		}
		firstGame = game
	} else {
		// ParsePGNLenient accepts any PGN regardless of which metadata tags are present;
		// handlePGN only needs the board sequence, not game metadata.
		games, parseErr := othello.ParsePGNLenient(text)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, parseErr)
			return
		}
		if len(games) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("no games found in PGN"))
			return
		}
		firstGame = games[0]
	}

	boards := firstGame.Boards()
	boardStrings := make([]string, len(boards))
	for i, b := range boards {
		boardStrings[i] = b.String()
	}

	writeJSON(w, http.StatusOK, pgnResponse{Boards: boardStrings})
}
