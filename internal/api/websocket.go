package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/lk16/flippy/internal/othello"
)

// wsIncoming is a message received from a websocket client.
type wsIncoming struct {
	ID    int             `json:"id"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// wsOutgoing is a message sent to a websocket client, correlated to the
// wsIncoming.ID that prompted it.
type wsOutgoing struct {
	ID   int `json:"id"`
	Data any `json:"data"`
}

// wsEvaluationRequest is the wsIncoming.Data shape for an
// "evaluation_request" event: a batch of (not necessarily normalized)
// board strings to look up.
type wsEvaluationRequest struct {
	Boards []string `json:"boards"`
}

// wsEvaluation is one evaluation in a wsEvaluationResponse, alongside the
// board string it's for (echoing the request, since it's a batch).
type wsEvaluation struct {
	Board string `json:"board"`
	evaluationResponse
}

// wsEvaluationResponse is the wsOutgoing.Data shape sent back for an
// "evaluation_request" event. Boards with no available evaluation (neither
// in the DB nor the minimax cache) are omitted rather than included with a
// placeholder.
type wsEvaluationResponse struct {
	Evaluations []wsEvaluation `json:"evaluations"`
}

// handleWebSocket handles GET /ws: a persistent connection for batched,
// low-latency evaluation lookups, for interactively exploring the board
// (e.g. every child of the position currently on screen) without a
// round trip per position.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx := r.Context()

	for {
		var incoming wsIncoming
		if err := wsjson.Read(ctx, conn, &incoming); err != nil {
			return
		}

		if incoming.Event != "evaluation_request" {
			continue
		}

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
	}
}

// lookupEvaluations looks up evaluations for a batch of (not necessarily
// normalized) board strings, skipping any that are malformed or have no
// available evaluation.
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
