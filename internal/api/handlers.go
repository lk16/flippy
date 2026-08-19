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

	writeJSON(w, http.StatusOK, jobResponse{Board: job.Board.String(), Level: job.Level})
}

// maxLevel bounds a submitted edax search level. It's well above any level flippy actually requests
// (see TargetLevel) but stays within the smallint column the evaluation is stored in, so an
// out-of-range value is a clean 400 rather than a Postgres error surfaced as a 500.
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

// checkReportedSearchParams logs when the depth/confidence a worker reports differ from what
// edax.SearchParams derives from (disc count, level). Neither is stored -- deriving them is only
// sound as long as the worker's edax agrees with the ported search_global_init, and this is the one
// place that comparison can still be made. A mismatch means an edax build that retuned the level
// table, so it's worth knowing about, but the score is still a real result: warn, don't reject.
// Workers that send neither field are silently accepted.
func checkReportedSearchParams(req jobResultRequest, discCount int) {
	if req.Depth == 0 && req.Confidence == 0 {
		return
	}

	depth, confidence := edax.SearchParams(discCount, req.Level)
	if req.Depth != depth || req.Confidence != confidence {
		log.Printf("board %s (%d discs) at level %d: worker reported %d@%d%%, edax's level table says %d@%d%%",
			req.Board, discCount, req.Level, req.Depth, req.Confidence, depth, confidence)
	}
}

// isBookQuality reports whether an evaluation is deep enough to belong in the boards table.
// Interactive (priority) analysis walks a board up from PriorityLevel in +2 rounds and every rung
// is its own job, so without this floor a single PGN review would write a dozen searches shallower
// than TargetLevel -- the depth the book is defined at -- into the DB, each of them a row
// ListLearnable then has to redo anyway. A search that already ran the game out counts as book
// quality whatever its level: no deeper search can change its score (edax.IsFinal).
//
// Enforced on every submission: ListLearnable jobs are handed out at TargetLevel(discCount) (see
// claimJob) and clear the floor by construction, but nothing stops a client from POSTing shallower
// results directly.
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

	discCount := normalized.CountDiscs()
	checkReportedSearchParams(req, discCount)

	eval := db.Evaluation{Level: req.Level, Score: req.Score}
	depth, confidence := edax.SearchParams(discCount, req.Level)

	// Always cache the result ephemerally so lookupEvaluation can find it regardless of DB eligibility.
	s.setAnalysisResult(r.Context(), normalized.String(), evaluationResponse{
		Level: req.Level, Depth: depth, Confidence: confidence, Score: req.Score,
		Source: evaluationSourceEdax,
	})

	// Check whether this job originated from the priority queue.
	isPriority, err := s.consumePriorityClaim(r.Context(), normalized.String())
	if err != nil {
		log.Printf("failed to check priority claim; treating as non-priority: %v", err)
	}

	savedToDB := false

	switch {
	case !isBookQuality(discCount, req.Level):
		// Below book quality: accepted but never persisted, priority or not. The ephemeral cache is
		// the only record; the frontend keeps asking one level deeper until a result that does
		// qualify comes back.
		//
		// A priority board this shallow may have no DB row at all, making it invisible to the book
		// learner; insert one (empty evaluation) so ListLearnable picks it up later. AddBoards is
		// insert-if-absent (ON CONFLICT DO NOTHING), so an existing evaluation is never downgraded.
		if isPriority && discCount <= book.MaxSavableDiscs {
			if err := s.repo.AddBoards(r.Context(), []othello.NormalizedBoard{normalized}); err != nil {
				log.Printf("failed to schedule priority board for learning: %v", err)
			}
		}
	case isPriority:
		if discCount <= book.MaxSavableDiscs {
			if saveErr := s.repo.SaveEvaluation(r.Context(), normalized, eval); saveErr != nil {
				if errors.Is(saveErr, db.ErrBoardNotFound) {
					// Board has no row yet; add one and retry.
					if addErr := s.repo.AddBoards(r.Context(), []othello.NormalizedBoard{normalized}); addErr != nil {
						log.Printf("failed to add priority board: %v", addErr)
					} else if saveErr2 := s.repo.SaveEvaluation(r.Context(), normalized, eval); saveErr2 != nil {
						log.Printf("failed to save priority evaluation after AddBoards: %v", saveErr2)
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
		// Too many discs: the ephemeral cache is the only record.
	default:
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
		depth, confidence := edax.SearchParams(board.CountDiscs(), eval.Level)
		return evaluationResponse{
			Level:      eval.Level,
			Depth:      depth,
			Confidence: confidence,
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

	if err := s.heartbeat(r.Context(), req.WorkerID, req.Hostname, req.GitCommit, req.Board); err != nil {
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

// statEntries turns per-(disc count, level) counts into per-(disc count, depth, confidence) counts.
// Levels are an implementation detail of how a search is requested: what a board is actually worth
// is the search it got, so levels describing the same search at the same disc count are merged.
// Unlearned boards (level 0) are reported as depth 0, confidence 0 rather than what the level table
// says a zero-level search would be, which would read as a full-confidence result.
// The result is ordered by disc count, then depth, then confidence.
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

// jobResponse is the JSON shape of a Job returned by GET /api/jobs.
type jobResponse struct {
	Board string `json:"board"`
	Level int    `json:"level"`
}

// jobResultRequest is the JSON body POSTed to /api/jobs/result. Depth and Confidence are what edax
// printed; neither is stored, since (disc count, level) already determines both. They are kept on
// the wire so checkReportedSearchParams can catch an edax whose level table differs from ours.
type jobResultRequest struct {
	WorkerID   string `json:"worker_id"`
	Board      string `json:"board"`
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
}

// releaseJobRequest is the JSON body POSTed to /api/jobs/release.
type releaseJobRequest struct {
	WorkerID string `json:"worker_id"`
	Board    string `json:"board"`
}

// heartbeatRequest is the JSON body POSTed to /api/workers/heartbeat. Board is the board the worker
// currently holds a claim on, if any, so the server can refresh that claim's TTL.
type heartbeatRequest struct {
	WorkerID  string `json:"worker_id"`
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
	Board     string `json:"board,omitempty"`
}

// evaluationSourceEdax marks an evaluationResponse as a directly-learned edax result.
const evaluationSourceEdax = "edax"

// evaluationSourceMinimax marks an evaluationResponse as backfilled from the internal/book cache.
const evaluationSourceMinimax = "minimax"

// evaluationSourceFinal marks an evaluationResponse as a board's actual final score.
const evaluationSourceFinal = "final"

// evaluationResponse is the JSON shape of an evaluation, returned by GET /api/boards. Depth and
// Confidence are derived from (disc count, level) via edax.SearchParams, not stored per board.
type evaluationResponse struct {
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
	Source     string `json:"source"`
}

// statEntry is one row of the GET /api/stats response: how many boards with DiscCount discs have
// been searched to Depth at Confidence percent. Levels are not reported -- two levels that search a
// board identically are one entry -- and unlearned boards report depth 0, confidence 0, since no
// search was run on them at all.
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
	Boards []string `json:"boards"`
}

// levelConfigResponse is the JSON body returned by GET /api/level-config. TargetLevels carries the
// whole disc-count tier table rather than a summary of it, so the frontend's notion of a board's
// target level matches EffectiveTargetLevel exactly -- a board whose target the frontend puts
// higher than the server's clamp can never reach it, leaving the PGN page reporting a search that
// never finishes.
type levelConfigResponse struct {
	PriorityLevel   int               `json:"priority_level"`
	MaxSavableDiscs int               `json:"max_savable_discs"`
	TargetLevels    []TargetLevelTier `json:"target_levels"`
}
