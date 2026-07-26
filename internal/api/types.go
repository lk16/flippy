package api

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
	WorkerID string `json:"worker_id"`
}

// evaluationResponse is the JSON shape of an evaluation, returned by GET
// /api/boards.
type evaluationResponse struct {
	Level      int `json:"level"`
	Depth      int `json:"depth"`
	Confidence int `json:"confidence"`
	Score      int `json:"score"`
}

// statEntry is one row of the GET /api/stats response.
type statEntry struct {
	DiscCount int `json:"disc_count"`
	Level     int `json:"level"`
	Count     int `json:"count"`
}
