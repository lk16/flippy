package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/othello"
)

const (
	clientTimeout = 1 * time.Second
)

type Client struct {
	// config contains details on how to connect to the server
	config *config.LearnClientConfig

	// hostname is the hostname of the machine running the client
	hostname string

	// gitCommit is the git commit hash of the client
	gitCommit string

	// clientID is the client ID of the client
	clientID string
}

func getGitCommit() (string, error) {
	commit, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed run git command, %w", err)
	}
	return strings.TrimSpace(string(commit)), nil
}

func NewClient(config *config.LearnClientConfig) (*Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	gitCommit, err := getGitCommit()
	if err != nil {
		return nil, fmt.Errorf("failed to get git commit: %w", err)
	}

	client := &Client{
		config:    config,
		hostname:  hostname,
		gitCommit: gitCommit,
	}

	slog.Debug("New APIClient created", "hostname", hostname, "git_commit", gitCommit)

	return client, nil
}

func (c *Client) logRequestAsCurl(req *http.Request) {
	// Do not build string if we're not logging it
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}

	var builder strings.Builder
	builder.WriteString("curl -X ")
	builder.WriteString(req.Method)
	builder.WriteString(" '")
	builder.WriteString(req.URL.String())
	builder.WriteString("'")

	for key, values := range req.Header {
		for _, value := range values {
			builder.WriteString(" -H '")
			builder.WriteString(strings.ToLower(key))
			builder.WriteString(": ")
			builder.WriteString(value)
			builder.WriteString("'")
		}
	}

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			slog.Error("Failed to read request body", "error", err)
		}

		if len(body) > 0 {
			builder.WriteString(" -d '")
			builder.WriteString(strings.ReplaceAll(string(body), "'", "'\\''"))
			builder.WriteString("'")
		}

		// Restore the original body
		req.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	slog.Debug("Sending request", "command", builder.String())
}

func (c *Client) logResponse(resp *http.Response) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}

	slog.Debug("Response", "status", resp.Status)

	if resp.Body != nil {
		// Create a copy of the body for logging
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Failed to read response body", "error", err)
			return
		}

		// Restore the original body
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		slog.Debug("Response body", "body", string(bodyBytes))
	}
}

func (c *Client) request(method string, path string, payload any, needsClientID bool) (*http.Response, error) {
	var body io.Reader

	if payload == nil {
		body = http.NoBody
	} else {
		buf := &bytes.Buffer{}
		err := json.NewEncoder(buf).Encode(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to encode payload: %w", err)
		}
		body = buf
	}

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.config.ServerURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("X-Token", c.config.Token)

	if needsClientID {
		err = c.ensureClientID()
		if err != nil {
			return nil, fmt.Errorf("failed to ensure client ID: %w", err)
		}

		req.Header.Set("X-Client-Id", c.clientID)
	}

	client := &http.Client{
		Timeout: clientTimeout,
	}

	c.logRequestAsCurl(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	c.logResponse(resp)

	if resp.StatusCode == http.StatusUnauthorized {
		slog.Debug("Unauthorized, requesting new client ID and trying again")
		c.clientID = ""
		return c.request(method, path, payload, needsClientID)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned unexpected status %v", resp.Status)
	}

	return resp, nil
}

func (c *Client) post(path string, payload any, needsClientID bool) (*http.Response, error) {
	return c.request("POST", path, payload, needsClientID)
}

func (c *Client) get(path string, needsClientID bool) (*http.Response, error) {
	return c.request("GET", path, nil, needsClientID)
}

func (c *Client) ensureClientID() error {
	if c.clientID != "" {
		slog.Debug("Already have client ID", "client_id", c.clientID)
		return nil
	}

	payload := RegisterRequest{
		Hostname:  c.hostname,
		GitCommit: c.gitCommit,
	}

	resp, err := c.post("/api/learn-clients/register", payload, false)
	if err != nil {
		return fmt.Errorf("failed to register learn client: %w", err)
	}

	defer resp.Body.Close()

	var parsed RegisterResponse
	err = json.NewDecoder(resp.Body).Decode(&parsed)
	if err != nil {
		return fmt.Errorf("failed to decode register response: %w", err)
	}

	c.clientID = parsed.ClientID
	return nil
}

func (c *Client) Heartbeat() error {
	resp, err := c.post("/api/learn-clients/heartbeat", nil, true)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	defer resp.Body.Close()
	return nil
}

var ErrNoJob = errors.New("no job available")

func (c *Client) GetJob() (Job, error) {
	resp, err := c.get("/api/learn-clients/job", true)
	if err != nil {
		return Job{}, fmt.Errorf("failed to get job: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Job{}, fmt.Errorf("failed to read job response body: %w", err)
	}

	if string(body) == "null" {
		return Job{}, ErrNoJob
	}

	var job Job
	err = json.Unmarshal(body, &job)
	if err != nil {
		return Job{}, fmt.Errorf("failed to unmarshal job response: %w", err)
	}

	return job, nil
}

func (c *Client) SubmitJobResult(payload EvaluationsPayload) error {
	resp, err := c.post("/api/positions/evaluations", payload, true)
	if err != nil {
		return fmt.Errorf("failed to submit job result: %w", err)
	}

	defer resp.Body.Close()

	return nil
}

func (c *Client) LookupPositions(positions []othello.NormalizedPosition) ([]Evaluation, error) {
	payload := LookupPositionsPayload{
		Positions: positions,
	}

	resp, err := c.post("/api/positions/lookup", payload, true)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup positions: %w", err)
	}

	defer resp.Body.Close()

	var evaluations []Evaluation
	err = json.NewDecoder(resp.Body).Decode(&evaluations)
	if err != nil {
		return nil, fmt.Errorf("failed to decode lookup positions response: %w", err)
	}

	return evaluations, nil
}

func (c *Client) SaveLearnedEvaluations(evaluations []Evaluation) error {
	if len(evaluations) == 0 {
		slog.Debug("No evaluations to save")
		return nil
	}

	payload := EvaluationsPayload{
		Evaluations: evaluations,
	}

	resp, err := c.post("/api/positions/evaluations", payload, true)
	if err != nil {
		return fmt.Errorf("failed to save learned evaluations: %w", err)
	}

	defer resp.Body.Close()

	return nil
}
