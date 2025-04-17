package models

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// RegisterRequest represents the payload for client registration
type RegisterRequest struct {
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
}

// RegisterResponse represents the response for client registration
type RegisterResponse struct {
	ClientID string `json:"client_id"`
}

// ClientStats represents client statistics
type ClientStats struct {
	ID                string             `json:"id"`
	Hostname          string             `json:"hostname"`
	GitCommit         string             `json:"git_commit"`
	PositionsComputed int                `json:"positions_computed"`
	LastActive        time.Time          `json:"last_active"`
	Position          NormalizedPosition `json:"position"`
}

// StatsResponse represents the response for client statistics
type StatsResponse struct {
	ActiveClients int           `json:"active_clients"`
	ClientStats   []ClientStats `json:"client_stats"`
}

// Job represents a work job for a client
type Job struct {
	Position NormalizedPosition `json:"position"`
	Level    int                `json:"level"`
}

// JobResult represents the result of a completed job
type JobResult struct {
	Evaluation Evaluation `json:"evaluation"`
}

// Evaluation represents an evaluation result
type Evaluation struct {
	Position   NormalizedPosition `json:"position"`
	Level      int                `json:"level"`
	Depth      int                `json:"depth"`
	Confidence float64            `json:"confidence"`
	Score      int                `json:"score"`
	BestMoves  []int              `json:"best_moves"`
}

// LookupPositionsPayload represents a request to look up positions
type LookupPositionsPayload struct {
	Positions []NormalizedPosition `json:"positions"`
}

// EvaluationsPayload represents a batch of evaluations to submit
type EvaluationsPayload struct {
	Evaluations []Evaluation `json:"evaluations"`
}

// ScanRow scans a database row into an Evaluation struct
func (e *Evaluation) ScanRow(rows *sqlx.Rows) error {
	var positionBytes []byte
	var bestMoves []int64

	if err := rows.Scan(&positionBytes, &e.Level, &e.Depth, &e.Confidence, &e.Score, pq.Array(&bestMoves)); err != nil {
		return fmt.Errorf("error scanning evaluation: %w", err)
	}

	var err error
	e.Position, err = NewNormalizedPositionFromBytes(positionBytes)
	if err != nil {
		return fmt.Errorf("error parsing position: %w", err)
	}

	// Convert int64 array to int array
	e.BestMoves = make([]int, len(bestMoves))
	for i, move := range bestMoves {
		e.BestMoves[i] = int(move)
	}

	return nil
}

type BookStats struct {
	DiscCount int `json:"disc_count"`
	Level     int `json:"level"`
	Count     int `json:"count"`
}
