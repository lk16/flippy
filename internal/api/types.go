package api

import "time"

// jobResponse is the JSON shape of a Job returned by GET /api/jobs.
type jobResponse struct {
	Board string `json:"board"`
	Level int    `json:"level"`
}

// jobResultRequest is the JSON body POSTed to /api/jobs/result.
type jobResultRequest struct {
	WorkerID   string `json:"worker_id"`
	Board      string `json:"board"`
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
}

// heartbeatRequest is the JSON body POSTed to /api/workers/heartbeat.
type heartbeatRequest struct {
	WorkerID  string `json:"worker_id"`
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
}

// evaluationSourceEdax marks an evaluationResponse as a directly-learned edax result.
const evaluationSourceEdax = "edax"

// evaluationSourceMinimax marks an evaluationResponse as backfilled from the internal/book cache.
const evaluationSourceMinimax = "minimax"

// evaluationSourceFinal marks an evaluationResponse as a board's actual final score.
const evaluationSourceFinal = "final"

// evaluationResponse is the JSON shape of an evaluation, returned by GET /api/boards.
type evaluationResponse struct {
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
	Source     string `json:"source"`
}

// statEntry is one row of the GET /api/stats response.
type statEntry struct {
	DiscCount int `json:"disc_count"`
	Level     int `json:"level"`
	Count     int `json:"count"`
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

// levelConfigResponse is the JSON body returned by GET /api/level-config.
type levelConfigResponse struct {
	PriorityLevel      int `json:"priority_level"`
	MaxSavableDiscs    int `json:"max_savable_discs"`
	LeafDiscs          int `json:"leaf_discs"`
	TargetLevelLeaf    int `json:"target_level_leaf"`
	TargetLevelNonLeaf int `json:"target_level_non_leaf"`
}
