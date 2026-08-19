package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

// wsIncoming is a message received from a websocket client.
type wsIncoming struct {
	ID    int             `json:"id"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// wsOutgoing is a message sent to a websocket client, correlated by ID to the request that prompted it.
type wsOutgoing struct {
	ID   int `json:"id"`
	Data any `json:"data"`
}

// wsEvaluationRequest is the wsIncoming.Data shape for "evaluation_request" and "analyze_request" events.
// Level is only used by "analyze_request"; if zero it defaults to PriorityLevel.
type wsEvaluationRequest struct {
	Boards []string `json:"boards"`
	Level  int      `json:"level,omitempty"`
}

// wsEvaluation is one evaluation in a wsEvaluationResponse, alongside the board string it's for.
type wsEvaluation struct {
	Board string `json:"board"`
	evaluationResponse
}

// wsEvaluationResponse is the wsOutgoing.Data shape sent back for evaluation and analysis events;
// boards with no available evaluation are omitted rather than included with a placeholder.
type wsEvaluationResponse struct {
	Evaluations []wsEvaluation `json:"evaluations"`
}

// registerConn allocates a connection ID and marks it live.
func (s *Server) registerConn() string {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	s.lastConnID++
	id := strconv.FormatInt(s.lastConnID, 10)
	s.liveConns[id] = struct{}{}
	return id
}

// unregisterConn marks a connection ID as no longer live.
func (s *Server) unregisterConn(id string) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	delete(s.liveConns, id)
}

// connLive reports whether a connection ID is still live.
func (s *Server) connLive(id string) bool {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	_, ok := s.liveConns[id]
	return ok
}

// handleWebSocket handles GET /ws: a persistent connection for batched evaluation lookups.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	connID := s.registerConn()
	defer s.unregisterConn(connID)

	ctx := r.Context()

	for {
		var incoming wsIncoming
		if err := wsjson.Read(ctx, conn, &incoming); err != nil {
			return
		}

		switch incoming.Event {
		case "evaluation_request":
			var req wsEvaluationRequest
			if err := json.Unmarshal(incoming.Data, &req); err != nil {
				continue
			}

			outgoing := wsOutgoing{
				ID:   incoming.ID,
				Data: wsEvaluationResponse{Evaluations: s.lookupEvaluations(ctx, req.Boards)},
			}
			if err := wsjson.Write(ctx, conn, outgoing); err != nil {
				return
			}

		case "analyze_request":
			var req wsEvaluationRequest
			if err := json.Unmarshal(incoming.Data, &req); err != nil {
				continue
			}

			level := req.Level
			if level <= 0 {
				level = PriorityLevel
			}

			s.handleAnalyzeRequest(ctx, req.Boards, level, connID)

			// Respond with whatever is already available, using the same shape as evaluation_request.
			outgoing := wsOutgoing{
				ID:   incoming.ID,
				Data: wsEvaluationResponse{Evaluations: s.lookupEvaluations(ctx, req.Boards)},
			}
			if err := wsjson.Write(ctx, conn, outgoing); err != nil {
				return
			}
		}
	}
}

// handleAnalyzeRequest enqueues boards not yet at the requested edax search level, tagging each
// queue entry with the requesting connection so the work is dropped if the requester disconnects.
func (s *Server) handleAnalyzeRequest(ctx context.Context, boardStrings []string, level int, connID string) {
	for _, bs := range boardStrings {
		board, err := othello.ParseBoard(bs)
		if err != nil {
			continue
		}

		// A forced-pass board (the player to move has no legal move) can't be searched by edax
		// directly — edax crashes on no-move positions. Its evaluation is the negation of the
		// position after the pass (see lookupPassEvaluation), so analyze that board instead; once
		// it resolves, the pass board's evaluation resolves too. This keeps all pass handling in the
		// backend: the frontend requests the pass board like any other and gets the negated result.
		// A game-over board (neither player can move) has a final score and needs no analysis.
		if !board.HasMoves() {
			passed, passErr := board.DoMove(othello.PassMove)
			if passErr != nil || !passed.HasMoves() {
				continue
			}
			board = passed
		}

		normalized := board.Normalize()
		discCount := normalized.CountDiscs()

		// Cap the requested level at the effective target for this board so a malicious client
		// cannot request arbitrarily deep searches and consume excessive worker CPU.
		clampedLevel := min(level, EffectiveTargetLevel(discCount))

		// Skip if the board already has a sufficient evaluation.
		// Minimax and final-score results are always sufficient regardless of requested level.
		// Edax results are sufficient if their level meets or exceeds what was requested, or if
		// the requested level describes a search they already are (searchAddsNothing).
		eval, ok, err := s.lookupEvaluation(ctx, board)
		if err == nil && ok {
			if eval.Source != evaluationSourceEdax || eval.Level >= clampedLevel {
				continue
			}
			if searchAddsNothing(eval, discCount, clampedLevel) {
				continue
			}
		}

		// Enqueue the normalized form so the claim key and queue entry are consistent.
		if err := s.enqueuePriority(ctx, normalized.String(), clampedLevel, connID); err != nil {
			slog.Warn("failed to enqueue priority board", "board", normalized.String(), "error", err)
		}
	}
}

// searchAddsNothing reports whether searching a board with discCount discs at level would return
// the evaluation it already has: either the level resolves to the same (depth, confidence) that
// evaluation was searched at -- levels are not one search each, and the endgame collapses whole
// runs of them onto the same solve -- or the stored result already ran the game out, which no
// level can improve on. Either way the answer is already in hand, so nothing is queued and the
// caller is served the stored evaluation.
func searchAddsNothing(eval evaluationResponse, discCount, level int) bool {
	if edax.IsFinal(discCount, eval.Level) {
		return true
	}

	depth, confidence := edax.SearchParams(discCount, level)
	return eval.Depth == depth && eval.Confidence == confidence
}

// lookupEvaluations looks up evaluations for a batch of board strings, skipping malformed or unevaluated ones.
func (s *Server) lookupEvaluations(ctx context.Context, boardStrings []string) []wsEvaluation {
	var results []wsEvaluation

	for _, bs := range boardStrings {
		board, err := othello.ParseBoard(bs)
		if err != nil {
			continue
		}

		eval, ok, err := s.lookupEvaluation(ctx, board)
		if err != nil || !ok {
			continue
		}

		results = append(results, wsEvaluation{Board: bs, evaluationResponse: eval})
	}

	return results
}
