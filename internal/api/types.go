package api

import "time"

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
