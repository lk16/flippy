package models

import (
	"time"
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
	Evaluation SerializedEvaluation `json:"evaluation"`
}

// Evaluation represents an evaluation in the database
type Evaluation struct {
	Position  NormalizedPosition `json:"position" db:"position"`
	Score     int                `json:"score" db:"score"`
	Level     int                `json:"level" db:"level"`
	DiscCount int                `json:"disc_count" db:"disc_count"`
}

// SerializedEvaluation represents an evaluation result
type SerializedEvaluation struct {
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
	Evaluations []SerializedEvaluation `json:"evaluations"`
}
