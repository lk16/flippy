package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/lk16/flippy/api/internal/repository"
	"github.com/lk16/flippy/api/internal/services"
)

const (
	evaluationTimeout = 2 * time.Second
)

type Handler struct {
	services *services.Services
	ws       *websocket.Conn
}

// NewHandler creates a new Handler.
func NewHandler(ws *websocket.Conn, services *services.Services) *Handler {
	return &Handler{services: services, ws: ws}
}

func (h *Handler) readMessage() (*Incoming, error) {
	var req Incoming

	msgType, msg, err := h.ws.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("ws read error: %w", err)
	}

	slog.Debug("read ws message", "msgType", msgType, "msg", msg)

	if msgType != websocket.TextMessage {
		return nil, fmt.Errorf("unexpected message type: %d", msgType)
	}

	if err = json.Unmarshal(msg, &req); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &req, nil
}

func (h *Handler) writeMessage(outgoing *Outgoing) error {
	msg, err := json.Marshal(outgoing)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	slog.Debug("write ws message", "msg", string(msg))

	if err = h.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}

func (h *Handler) handleMessage(req *Incoming) (*Outgoing, error) {
	if req.Event == "" {
		return nil, errors.New("event field is either empty or missing")
	}

	switch req.Event {
	case "evaluation_request":
		return h.handleEvaluationRequest(req)
	default:
		return nil, fmt.Errorf("unknown event: %s", req.Event)
	}
}

// Handle handles the websocket connection.
func (h *Handler) Handle() error {
	for {
		req, err := h.readMessage()
		if err != nil {
			return fmt.Errorf("ws read error: %w", err)
		}

		respData, err := h.handleMessage(req)
		if err != nil {
			return fmt.Errorf("ws handle error: %w", err)
		}

		if err = h.writeMessage(respData); err != nil {
			return fmt.Errorf("ws write error: %w", err)
		}
	}
}

func (h *Handler) handleEvaluationRequest(req *Incoming) (*Outgoing, error) {
	var reqData EvaluationRequest
	if err := json.Unmarshal(req.Data, &reqData); err != nil {
		return nil, fmt.Errorf("ws evaluation request unmarshal error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), evaluationTimeout)
	defer cancel()

	repo := repository.NewEvaluationRepositoryFromServices(h.services)

	evaluations, err := repo.LookupPositions(ctx, reqData.Positions)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup positions: %w", err)
	}

	outgoing := &Outgoing{
		ID: req.ID,
		Data: EvaluationResponse{
			Evaluations: evaluations,
		},
	}

	return outgoing, nil
}
