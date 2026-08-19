// Package worker implements the job loop that claims positions from the API,
// evaluates them with edax, and submits the results back.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/lk16/flippy/internal/edax"
)

// Job is a position to evaluate, and the level to search it at.
type Job struct {
	Position string
	Level    int
}

// Client talks to the flippy API server on behalf of a single worker identity.
type Client struct {
	baseURL    string
	workerID   string
	hostname   string
	gitCommit  string
	httpClient *http.Client
}

// NewClient returns a Client for the API server at baseURL, identifying as workerID.
func NewClient(baseURL, workerID, hostname, gitCommit string) *Client {
	return &Client{
		baseURL:    baseURL,
		workerID:   workerID,
		hostname:   hostname,
		gitCommit:  gitCommit,
		httpClient: http.DefaultClient,
	}
}

// jobResponse is the JSON shape of GET /api/jobs's response body. The API calls a position a
// "board" on the wire, so the json tags here and below are the only place that name survives.
type jobResponse struct {
	Position string `json:"board"`
	Level    int    `json:"level"`
}

// GetJob claims one available job; ok is false when the server has none.
func (c *Client) GetJob(ctx context.Context) (job Job, ok bool, err error) {
	target := fmt.Sprintf("%s/api/jobs?worker_id=%s", c.baseURL, url.QueryEscape(c.workerID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Job{}, false, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Job{}, false, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return Job{}, false, nil
	case http.StatusOK:
		var jr jobResponse
		if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
			return Job{}, false, fmt.Errorf("failed to decode job response: %w", err)
		}
		return Job(jr), true, nil
	default:
		return Job{}, false, fmt.Errorf("unexpected status %d from GET /api/jobs", resp.StatusCode)
	}
}

// jobResultRequest is the JSON body POSTed to /api/jobs/result.
type jobResultRequest struct {
	WorkerID   string `json:"worker_id"`
	Position   string `json:"board"`
	Level      int    `json:"level"`
	Depth      int    `json:"depth"`
	Confidence int    `json:"confidence"`
	Score      int    `json:"score"`
}

// SubmitJobResult submits eval, computed at level, for position.
func (c *Client) SubmitJobResult(ctx context.Context, position string, level int, eval edax.Evaluation) error {
	body := jobResultRequest{
		WorkerID:   c.workerID,
		Position:   position,
		Level:      level,
		Depth:      eval.Depth,
		Confidence: eval.Confidence,
		Score:      eval.Score,
	}
	return c.post(ctx, "/api/jobs/result", body)
}

// releaseJobRequest is the JSON body POSTed to /api/jobs/release.
type releaseJobRequest struct {
	WorkerID string `json:"worker_id"`
	Position string `json:"board"`
}

// ReleaseJob releases this worker's claim on position without submitting a result, e.g. when shutting
// down with the job still queued or in flight, so another worker doesn't have to wait out claimTTL.
func (c *Client) ReleaseJob(ctx context.Context, position string) error {
	return c.post(ctx, "/api/jobs/release", releaseJobRequest{WorkerID: c.workerID, Position: position})
}

// heartbeatRequest is the JSON body POSTed to /api/workers/heartbeat.
type heartbeatRequest struct {
	WorkerID  string `json:"worker_id"`
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
	Position  string `json:"board,omitempty"`
}

// Heartbeat reports this worker as active, refreshing its claim on position ("" for none).
func (c *Client) Heartbeat(ctx context.Context, position string) error {
	return c.post(ctx, "/api/workers/heartbeat", heartbeatRequest{
		WorkerID:  c.workerID,
		Hostname:  c.hostname,
		GitCommit: c.gitCommit,
		Position:  position,
	})
}

// post sends body as JSON to path, erroring unless the server responds 200 OK.
func (c *Client) post(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from POST %s", resp.StatusCode, path)
	}
	return nil
}
