package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
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

// handleGetJob handles GET /api/jobs: claims and returns one available job, or 204 if none.
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

	writeJSON(w, http.StatusOK, jobResponse{Position: job.Position.String(), Level: job.Level})
}

// maxLevel bounds a submitted search level: above anything flippy requests, but within the
// smallint column, so an out-of-range value is a 400 rather than a Postgres error.
const maxLevel = 60

// validateJobResult checks that a submitted evaluation's stored values are within sane bounds.
func validateJobResult(req jobResultRequest) error {
	if req.Level <= 0 || req.Level > maxLevel {
		return fmt.Errorf("level must be between 1 and %d", maxLevel)
	}
	if req.Score < -64 || req.Score > 64 {
		return errors.New("score out of range")
	}
	return nil
}

// checkReportedSearchParams logs when a worker's reported depth/confidence disagree with
// edax.SearchParams -- the sign of an edax build with a retuned level table. The score is still a
// real result: log, don't reject. Workers that send neither field are silently accepted.
func checkReportedSearchParams(req jobResultRequest, discCount int) {
	if req.Depth == 0 && req.Confidence == 0 {
		return
	}

	depth, confidence := edax.SearchParams(discCount, req.Level)
	if req.Depth != depth || req.Confidence != confidence {
		log.Printf("position %s (%d discs) at level %d: worker reported %d@%d%%, edax's level table says %d@%d%%",
			req.Position, discCount, req.Level, req.Depth, req.Confidence, depth, confidence)
	}
}

// isBookQuality reports whether an evaluation is deep enough for the boards table: at least the
// position's target level, or a search that ran the game out, which no deeper search can improve on.
// Enforced on every submission so interactive analysis's shallow rungs never enter the book.
func isBookQuality(discCount, level int) bool {
	return level >= TargetLevel(discCount) || edax.IsFinal(discCount, level)
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

	position, err := othello.ParsePosition(req.Position)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid board: %w", err))
		return
	}

	normalized, err := othello.NewNormalizedPosition(position)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("board must be normalized: %w", err))
		return
	}

	if err := validateJobResult(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	discCount := normalized.CountDiscs()
	checkReportedSearchParams(req, discCount)

	eval := db.Evaluation{Level: req.Level, Score: req.Score}
	depth, confidence := edax.SearchParams(discCount, req.Level)

	// Always cache the result ephemerally so lookupEvaluation can find it regardless of DB eligibility.
	s.setAnalysisResult(r.Context(), normalized.String(), evaluationResponse{
		Level: req.Level, Depth: depth, Confidence: confidence, Score: req.Score,
		Source: evaluationSourceEdax,
	})

	isPriority, err := s.consumePriorityClaim(r.Context(), normalized.String())
	if err != nil {
		log.Printf("failed to check priority claim; treating as non-priority: %v", err)
	}

	savedToDB := false

	switch {
	case !isBookQuality(discCount, req.Level):
		// Below book quality: accepted but never persisted; the ephemeral cache is the only record.
		// A savable priority position still gets an empty-evaluation row so ListLearnable finds it
		// later (AddPositions never downgrades an existing row).
		if isPriority && discCount <= book.MaxSavableDiscs {
			if inserted, err := s.repo.AddPositionsInserted(r.Context(), []othello.NormalizedPosition{normalized}); err != nil {
				log.Printf("failed to schedule priority position for learning: %v", err)
			} else {
				s.bookStatsRecordInsert(r.Context(), discCount, inserted)
			}
		}
	case isPriority:
		if discCount <= book.MaxSavableDiscs {
			if outcome, saveErr := s.repo.SaveEvaluationOutcome(r.Context(), normalized, eval); saveErr != nil {
				if errors.Is(saveErr, db.ErrPositionNotFound) {
					// Position has no row yet; add one and retry.
					if inserted, addErr := s.repo.AddPositionsInserted(r.Context(), []othello.NormalizedPosition{normalized}); addErr != nil {
						log.Printf("failed to add priority position: %v", addErr)
					} else if outcome2, saveErr2 := s.repo.SaveEvaluationOutcome(r.Context(), normalized, eval); saveErr2 != nil {
						s.bookStatsRecordInsert(r.Context(), discCount, inserted)
						log.Printf("failed to save priority evaluation after AddPositions: %v", saveErr2)
					} else {
						savedToDB = true
						s.bookStatsRecordInsert(r.Context(), discCount, inserted)
						s.bookStatsRecordSave(r.Context(), discCount, outcome2, req.Level)
					}
				} else {
					writeError(w, http.StatusInternalServerError, saveErr)
					return
				}
			} else {
				savedToDB = true
				s.bookStatsRecordSave(r.Context(), discCount, outcome, req.Level)
			}
		}
		// Too many discs: the ephemeral cache is the only record.
	default:
		// ErrPositionNotFound is a real bug here: every ListLearnable position already has a row.
		outcome, err := s.repo.SaveEvaluationOutcome(r.Context(), normalized, eval)
		if err != nil {
			if errors.Is(err, db.ErrPositionNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		savedToDB = true
		s.bookStatsRecordSave(r.Context(), discCount, outcome, req.Level)
	}

	if savedToDB {
		// Only a leaf-disc-count save can change the minimax backfill. Bump the shared book
		// version so every replica rebuilds (see RunCacheInvalidation) instead of rebuilding
		// inline, which stalled all workers for the duration of a full leaf scan. On error the
		// worker gets a 500; the position stays learnable, so the bump is eventually retried.
		if discCount == book.LeafDiscs {
			if err := s.bumpBookVersion(r.Context()); err != nil {
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

// handleReleaseJob handles POST /api/jobs/release: releases a worker's claim on a position it didn't
// finish, so another worker can pick it up without waiting out claimTTL.
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

	position, err := othello.ParsePosition(req.Position)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid board: %w", err))
		return
	}

	normalized, err := othello.NewNormalizedPosition(position)
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

// lookupEvaluation returns position's evaluation from the DB, minimax cache, or ephemeral analysis
// cache; ok is false if none has a real (learned) result.
func (s *Server) lookupEvaluation(ctx context.Context, position othello.Position) (evaluationResponse, bool, error) {
	if !position.HasMoves() {
		return s.lookupPassEvaluation(ctx, position)
	}

	eval, err := s.repo.GetPosition(ctx, position)
	if err == nil && eval.IsLearned() {
		depth, confidence := edax.SearchParams(position.CountDiscs(), eval.Level)
		return evaluationResponse{
			Level:      eval.Level,
			Depth:      depth,
			Confidence: confidence,
			Score:      eval.Score,
			Source:     evaluationSourceEdax,
		}, true, nil
	}
	if err != nil && !errors.Is(err, db.ErrPositionNotFound) {
		return evaluationResponse{}, false, err
	}

	if score, ok := s.cache.Get(position); ok {
		return evaluationResponse{Score: score, Source: evaluationSourceMinimax}, true, nil
	}

	// Covers priority-computed evaluations that never reach the DB (too many discs, below target).
	if cached, ok, err := s.getAnalysisResult(ctx, position.Normalize().String()); err == nil && ok {
		return cached, true, nil
	}

	return evaluationResponse{}, false, nil
}

// lookupPassEvaluation handles a forced-pass position: negates the other player's evaluation, or returns
// the final score if the pass ends the game.
func (s *Server) lookupPassEvaluation(ctx context.Context, position othello.Position) (evaluationResponse, bool, error) {
	passed, err := position.DoMove(othello.PassMove)
	if err != nil {
		return evaluationResponse{}, false, err
	}

	if !passed.HasMoves() {
		return evaluationResponse{Score: position.FinalScore(), Source: evaluationSourceFinal}, true, nil
	}

	eval, ok, err := s.lookupEvaluation(ctx, passed)
	if err != nil || !ok {
		return evaluationResponse{}, ok, err
	}

	eval.Score = -eval.Score
	return eval, true, nil
}

// handleGetBoard handles GET /api/boards?position=<position string>: looks up a position's evaluation.
func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	boardParam := r.URL.Query().Get("board")
	if boardParam == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing board"))
		return
	}

	position, err := othello.ParsePosition(boardParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid board: %w", err))
		return
	}

	eval, ok, err := s.lookupEvaluation(r.Context(), position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, db.ErrPositionNotFound)
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

	if err := s.heartbeat(r.Context(), req.WorkerID, req.Hostname, req.GitCommit, req.Position); err != nil {
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

// handleStats handles GET /api/stats: returns position counts per (disc count, depth, confidence)
// cell, served from the periodically rebuilt book_stats hash (see RunBookStatsRefresh). If the hash
// is missing (Redis flushed, first boot race), the DB is queried directly.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	entries, ok, err := s.getBookStats(r.Context())
	if err == nil && ok {
		writeJSON(w, http.StatusOK, entries)
		return
	}

	stats, err := s.repo.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, statEntries(stats))
}

// statEntries merges per-(disc count, level) counts into sorted per-(disc count, depth, confidence)
// entries: levels describing the same search are one entry, and unlearned positions (level 0) report
// depth 0, confidence 0 rather than what the level table would claim for them.
func statEntries(stats []db.LevelStat) []statEntry {
	counts := make(map[statEntry]int, len(stats))
	for _, stat := range stats {
		key := statEntry{DiscCount: stat.DiscCount}
		if stat.Level > 0 {
			key.Depth, key.Confidence = edax.SearchParams(stat.DiscCount, stat.Level)
		}
		counts[key] += stat.Count
	}

	entries := make([]statEntry, 0, len(counts))
	for key, count := range counts {
		key.Count = count
		entries = append(entries, key)
	}

	sortStatEntries(entries)
	return entries
}

// sortStatEntries orders entries by disc count, then depth, then confidence.
func sortStatEntries(entries []statEntry) {
	slices.SortFunc(entries, func(a, b statEntry) int {
		return cmp.Or(
			cmp.Compare(a.DiscCount, b.DiscCount),
			cmp.Compare(a.Depth, b.Depth),
			cmp.Compare(a.Confidence, b.Confidence),
		)
	})
}

// handleLevelConfig handles GET /api/level-config: returns the constants the frontend needs to
// determine how many level-increment rounds to request per position.
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
// returns the position sequence as strings. Only the first game in a multi-game PGN is used.
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

	positions := firstGame.Positions()
	positionStrings := make([]string, len(positions))
	for i, b := range positions {
		positionStrings[i] = b.String()
	}

	writeJSON(w, http.StatusOK, pgnResponse{Positions: positionStrings})
}

// jobResponse is the JSON shape of a Job returned by GET /api/jobs. The HTTP API calls a position
// a "board" -- in its routes, its query parameters and the json tags below -- and that is the only
// place the older name survives.
type jobResponse struct {
	Position string `json:"board"`
	Level    int    `json:"level"`
}

// jobResultRequest is the JSON body POSTed to /api/jobs/result. Depth and Confidence are not
// stored; they stay on the wire so checkReportedSearchParams can catch a mismatched edax build.
type jobResultRequest struct {
	WorkerID   string `json:"worker_id"`
	Position   string `json:"board"`
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
}

// releaseJobRequest is the JSON body POSTed to /api/jobs/release.
type releaseJobRequest struct {
	WorkerID string `json:"worker_id"`
	Position string `json:"board"`
}

// heartbeatRequest is the JSON body POSTed to /api/workers/heartbeat. Position is the position the worker
// currently holds a claim on, if any, so the server can refresh that claim's TTL.
type heartbeatRequest struct {
	WorkerID  string `json:"worker_id"`
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
	Position  string `json:"board,omitempty"`
}

// evaluationSourceEdax marks an evaluationResponse as a directly-learned edax result.
const evaluationSourceEdax = "edax"

// evaluationSourceMinimax marks an evaluationResponse as backfilled from the internal/book cache.
const evaluationSourceMinimax = "minimax"

// evaluationSourceFinal marks an evaluationResponse as a position's actual final score.
const evaluationSourceFinal = "final"

// evaluationResponse is the JSON shape of an evaluation, returned by GET /api/boards. Depth and
// Confidence are derived via edax.SearchParams, not stored per position.
type evaluationResponse struct {
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
	Source     string `json:"source"`
}

// statEntry is one row of the GET /api/stats response: how many positions with DiscCount discs have
// been searched to Depth at Confidence percent.
type statEntry struct {
	DiscCount  int `json:"disc_count"`
	Depth      int `json:"depth"`
	Confidence int `json:"confidence"`
	Count      int `json:"count"`
}

// workerResponse is one entry of the GET /api/workers response.
type workerResponse struct {
	ID                string    `json:"id"`
	Hostname          string    `json:"hostname"`
	GitCommit         string    `json:"git_commit"`
	PositionsComputed int       `json:"positions_computed"`
	LastActive        time.Time `json:"last_active"`
}

// pgnResponse is the JSON body returned by POST /api/pgn.
type pgnResponse struct {
	Positions []string `json:"boards"`
}

// levelConfigResponse is the JSON body returned by GET /api/level-config. TargetLevels carries the
// whole tier table so the frontend computes exactly the targets the server enforces.
type levelConfigResponse struct {
	PriorityLevel   int               `json:"priority_level"`
	MaxSavableDiscs int               `json:"max_savable_discs"`
	TargetLevels    []TargetLevelTier `json:"target_levels"`
}
