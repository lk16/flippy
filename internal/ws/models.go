package ws

import (
	"encoding/json"

	"github.com/lk16/flippy/api/internal/models"
)

type Incoming struct {
	Event string          `json:"event"`
	ID    int             `json:"id"`
	Data  json.RawMessage `json:"data"`
}

type Outgoing struct {
	ID   int `json:"id"`
	Data any `json:"data"`
}

type EvaluationRequest struct {
	Positions []models.NormalizedPosition `json:"positions"`
}

type EvaluationResponse struct {
	Evaluations []models.Evaluation `json:"evaluations"`
}
