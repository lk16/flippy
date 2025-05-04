package models

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lk16/flippy/api/internal/config"
)

// RegisterRequest represents the payload for client registration.
type RegisterRequest struct {
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
}

// RegisterResponse represents the response for client registration.
type RegisterResponse struct {
	ClientID string `json:"client_id"`
}

// ClientStats represents client statistics.
type ClientStats struct {
	ID                string             `json:"id"`
	Hostname          string             `json:"hostname"`
	GitCommit         string             `json:"git_commit"`
	PositionsComputed int                `json:"positions_computed"`
	LastActive        time.Time          `json:"last_active"`
	Position          NormalizedPosition `json:"position"`
}

// StatsResponse represents the response for client statistics.
type StatsResponse struct {
	ActiveClients int           `json:"active_clients"`
	ClientStats   []ClientStats `json:"client_stats"`
}

// Job represents a work job for a client.
type Job struct {
	Position NormalizedPosition `json:"position"`
	Level    int                `json:"level"`
}

// JobResult represents the result of a completed job.
type JobResult struct {
	Evaluation      Evaluation `json:"evaluation"`
	ComputationTime float64    `json:"computation_time"`
}

// Evaluation represents an evaluation result.
type Evaluation struct {
	Position   NormalizedPosition `json:"position"   db:"position"`
	Level      int                `json:"level"      db:"level"`
	Depth      int                `json:"depth"      db:"depth"`
	Confidence int                `json:"confidence" db:"confidence"`
	Score      int                `json:"score"      db:"score"`
	BestMoves  BestMoves          `json:"best_moves" db:"best_moves"`
}

func (e *Evaluation) Validate() error {
	if !e.Position.IsDBSavable() {
		return errors.New("position is not savable")
	}

	if e.Level < config.MinBookLearnLevel {
		return fmt.Errorf("level is below the minimum learn level of %d", config.MinBookLearnLevel)
	}

	if e.Depth < 0 || e.Depth > 60 {
		return errors.New("depth is out of range")
	}

	validConfidences := []int{73, 87, 95, 98, 99, 100}
	isValidConfidence := false
	for _, c := range validConfidences {
		if e.Confidence == c {
			isValidConfidence = true
			break
		}
	}
	if !isValidConfidence {
		return fmt.Errorf("confidence must be one of: %v", validConfidences)
	}

	if e.Score < -64 || e.Score > 64 {
		return errors.New("score must be between -64 and 64")
	}

	return e.Position.ValidateBestMoves(e.BestMoves)
}

// BestMoves is a slice of BestMove that implements sql.Scanner.
type BestMoves []int

// Scan implements the sql.Scanner interface for BestMoves.
func (b *BestMoves) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into BestMoves", value)
	}

	if bytes == nil {
		return errors.New("cannot scan nil into BestMoves")
	}

	s := string(bytes)

	// We should have a string that looks like "{1,2,3}"
	s = strings.Trim(s, "{}")

	parts := strings.Split(s, ",")

	moves := make([]int, len(parts))
	for i, part := range parts {
		move, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("cannot convert %s to int: %w", part, err)
		}
		moves[i] = move
	}
	*b = moves

	return nil
}

// LookupPositionsPayload represents a request to look up positions.
type LookupPositionsPayload struct {
	Positions []NormalizedPosition `json:"positions"`
}

// EvaluationsPayload represents a batch of evaluations to submit.
type EvaluationsPayload struct {
	Evaluations []Evaluation `json:"evaluations"`
}

// Validate validates the evaluations payload.
func (p *EvaluationsPayload) Validate() error {
	if len(p.Evaluations) == 0 {
		return errors.New("evaluations is empty")
	}

	for _, evaluation := range p.Evaluations {
		if err := evaluation.Validate(); err != nil {
			return err
		}
	}

	return nil
}

type BookStats struct {
	DiscCount int `json:"disc_count"`
	Level     int `json:"level"`
	Count     int `json:"count"`
}

type VersionResponse struct {
	Commit string `json:"commit"`
}
