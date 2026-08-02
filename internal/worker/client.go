// Package worker implements the job loop that claims boards from the API,
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

// Job is a board to evaluate, and the level to search it at.
type Job struct {
	Board string
	Level int
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

// jobResponse is the JSON shape of GET /api/jobs's response body.
type jobResponse struct {
	Board string `json:"board"`
	Level int    `json:"level"`
}

// GetJobs claims up to count available jobs; it may return fewer than count, including none.
func (c *Client) GetJobs(ctx context.Context, count int) ([]Job, error) {
	target := fmt.Sprintf("%s/api/jobs?worker_id=%s&count=%d", c.baseURL, url.QueryEscape(c.workerID), count)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var jrs []jobResponse
		if err := json.NewDecoder(resp.Body).Decode(&jrs); err != nil {
			return nil, fmt.Errorf("failed to decode jobs response: %w", err)
		}
		jobs := make([]Job, len(jrs))
		for i, jr := range jrs {
			jobs[i] = Job(jr)
		}
		return jobs, nil
	default:
		return nil, fmt.Errorf("unexpected status %d from GET /api/jobs", resp.StatusCode)
	}
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

// SubmitJobResult submits eval, computed at level, for board.
func (c *Client) SubmitJobResult(ctx context.Context, board string, level int, eval edax.Evaluation) error {
	body := jobResultRequest{
		WorkerID:   c.workerID,
		Board:      board,
		Level:      level,
		Depth:      eval.Depth,
		Confidence: eval.Confidence,
		Score:      eval.Score,
	}
	return c.post(ctx, "/api/jobs/result", body)
}

// heartbeatRequest is the JSON body POSTed to /api/workers/heartbeat.
type heartbeatRequest struct {
	WorkerID  string `json:"worker_id"`
	Hostname  string `json:"hostname"`
	GitCommit string `json:"git_commit"`
}

// Heartbeat reports this worker as active and refreshes its job claim, if it has one.
func (c *Client) Heartbeat(ctx context.Context) error {
	return c.post(ctx, "/api/workers/heartbeat", heartbeatRequest{
		WorkerID:  c.workerID,
		Hostname:  c.hostname,
		GitCommit: c.gitCommit,
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
