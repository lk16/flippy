package api

import (
	"context"
	"encoding/json"
	"log"
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
	Positions []string `json:"boards"`
	Level     int      `json:"level,omitempty"`
}

// wsEvaluation is one evaluation in a wsEvaluationResponse, alongside the position string it's for.
type wsEvaluation struct {
	Position string `json:"board"`
	evaluationResponse
}

// wsEvaluationResponse is the wsOutgoing.Data shape sent back for evaluation and analysis events;
// positions with no available evaluation are omitted rather than included with a placeholder.
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
				Data: wsEvaluationResponse{Evaluations: s.lookupEvaluations(ctx, req.Positions)},
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

			s.handleAnalyzeRequest(ctx, req.Positions, level, connID)

			// Respond with whatever is already available, using the same shape as evaluation_request.
			outgoing := wsOutgoing{
				ID:   incoming.ID,
				Data: wsEvaluationResponse{Evaluations: s.lookupEvaluations(ctx, req.Positions)},
			}
			if err := wsjson.Write(ctx, conn, outgoing); err != nil {
				return
			}
		}
	}
}

// handleAnalyzeRequest enqueues positions not yet at the requested edax search level, tagging each
// queue entry with the requesting connection so the work is dropped if the requester disconnects.
func (s *Server) handleAnalyzeRequest(ctx context.Context, positionStrings []string, level int, connID string) {
	for _, bs := range positionStrings {
		position, err := othello.ParsePosition(bs)
		if err != nil {
			continue
		}

		// edax crashes on a position with no legal move: analyze the post-pass position instead (its
		// negation is the pass position's evaluation, see lookupPassEvaluation). A game-over position has
		// a final score and needs no analysis.
		if !position.HasMoves() {
			passed, passErr := position.DoMove(othello.PassMove)
			if passErr != nil || !passed.HasMoves() {
				continue
			}
			position = passed
		}

		normalized := position.Normalize()
		discCount := normalized.CountDiscs()

		// Cap the level so a malicious client cannot request arbitrarily deep searches.
		clampedLevel := min(level, EffectiveTargetLevel(discCount))

		// Skip positions whose stored evaluation already answers the request.
		eval, ok, err := s.lookupEvaluation(ctx, position)
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
			log.Printf("failed to enqueue priority position %s: %v", normalized.String(), err)
		}
	}
}

// searchAddsNothing reports whether a search at level would just repeat the stored evaluation:
// the level maps to the same (depth, confidence) it was searched at, or the stored result already
// ran the game out, which no level can improve on.
func searchAddsNothing(eval evaluationResponse, discCount, level int) bool {
	if edax.IsFinal(discCount, eval.Level) {
		return true
	}

	depth, confidence := edax.SearchParams(discCount, level)
	return eval.Depth == depth && eval.Confidence == confidence
}

// lookupEvaluations looks up evaluations for a batch of position strings, skipping malformed or unevaluated ones.
func (s *Server) lookupEvaluations(ctx context.Context, positionStrings []string) []wsEvaluation {
	var results []wsEvaluation

	for _, bs := range positionStrings {
		position, err := othello.ParsePosition(bs)
		if err != nil {
			continue
		}

		eval, ok, err := s.lookupEvaluation(ctx, position)
		if err != nil || !ok {
			continue
		}

		results = append(results, wsEvaluation{Position: bs, evaluationResponse: eval})
	}

	return results
}
