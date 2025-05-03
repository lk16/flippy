package book

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	clientTimeout = 1 * time.Second
)

type APIClient struct {
	// config contains details on how to connect to the server
	config *config.LearnClientConfig

	// hostname is the hostname of the machine running the client
	hostname string

	// gitCommit is the git commit hash of the client
	gitCommit string

	// verbose is whether to log more details, useful for debugging
	verbose bool

	// clientID is the client ID of the client
	clientID string
}

func getGitCommit() string {
	commit, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		log.Fatalf("Failed to get git commit: %v", err)
	}
	return strings.TrimSpace(string(commit))
}

func NewAPIClient(config *config.LearnClientConfig, verbose bool) *APIClient {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("Failed to get hostname: %v", err)
	}
	gitCommit := getGitCommit()

	client := &APIClient{
		config:    config,
		hostname:  hostname,
		gitCommit: gitCommit,
		verbose:   verbose,
	}

	client.logVerbose("New APIClient created")
	client.logVerbose("Hostname: %s", hostname)
	client.logVerbose("Git commit: %s", gitCommit)

	return client
}

func (c *APIClient) logVerbose(format string, args ...any) {
	if c.verbose {
		log.Printf(format, args...)
	}
}

func (c *APIClient) logRequestAsCurl(request *http.Request) {
	// Do not build string if we're not logging it
	if !c.verbose {
		return
	}

	var builder strings.Builder
	builder.WriteString("curl -X ")
	builder.WriteString(request.Method)
	builder.WriteString(" '")
	builder.WriteString(request.URL.String())
	builder.WriteString("'")

	for key, values := range request.Header {
		for _, value := range values {
			builder.WriteString(" -H '")
			builder.WriteString(strings.ToLower(key))
			builder.WriteString(": ")
			builder.WriteString(value)
			builder.WriteString("'")
		}
	}

	if request.Body != nil {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			c.logVerbose("Failed to read request body: %v", err)
		}

		if len(body) > 0 {
			builder.WriteString(" -d '")
			builder.WriteString(strings.ReplaceAll(string(body), "'", "'\\''"))
			builder.WriteString("'")
		}

		// Restore the original body
		request.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	c.logVerbose("%s", builder.String())
}

func (c *APIClient) logResponse(response *http.Response) {
	if !c.verbose {
		return
	}

	c.logVerbose("Response: %v", response.Status)

	if response.Body != nil {
		// Create a copy of the body for logging
		bodyBytes, err := io.ReadAll(response.Body)
		if err != nil {
			c.logVerbose("Failed to read response body: %v", err)
			return
		}

		// Restore the original body
		response.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		c.logVerbose("Response body: %s", string(bodyBytes))
	}
}

func (c *APIClient) request(method string, path string, payload any, needsClientID bool) (*http.Response, error) {

	var body io.Reader

	if payload == nil {
		body = http.NoBody
	} else {
		buf := &bytes.Buffer{}
		err := json.NewEncoder(buf).Encode(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to encode payload: %v", err)
		}
		body = buf
	}

	request, err := http.NewRequest(method, c.config.ServerURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	request.Header.Set("x-token", c.config.Token)

	if needsClientID {
		err := c.ensureClientID()
		if err != nil {
			return nil, fmt.Errorf("failed to ensure client ID: %v", err)
		}

		request.Header.Set("x-client-id", c.clientID)
	}

	client := &http.Client{
		Timeout: clientTimeout,
	}

	c.logRequestAsCurl(request)

	response, err := client.Do(request)
	if err != nil {
		c.logVerbose("Failed to send request: %v", err)
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	c.logResponse(response)

	if response.StatusCode == http.StatusUnauthorized {
		c.logVerbose("Unauthorized, requesting new client ID and trying again")
		c.clientID = ""
		return c.request(method, path, payload, needsClientID)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned unexpected status %v", response.Status)
	}

	return response, nil
}

func (c *APIClient) post(path string, payload any, needsClientID bool) (*http.Response, error) {
	return c.request("POST", path, payload, needsClientID)
}

func (c *APIClient) get(path string, needsClientID bool) (*http.Response, error) {
	return c.request("GET", path, nil, needsClientID)
}

func (c *APIClient) ensureClientID() error {
	if c.clientID != "" {
		c.logVerbose("Client ID already exists: %s", c.clientID)
		return nil
	}

	payload := models.RegisterRequest{
		Hostname:  c.hostname,
		GitCommit: c.gitCommit,
	}

	response, err := c.post("/api/learn-clients/register", payload, false)
	if err != nil {
		c.logVerbose("Failed to register learn client: %v", err)
		return fmt.Errorf("failed to register learn client: %v", err)
	}

	var parsed models.RegisterResponse
	err = json.NewDecoder(response.Body).Decode(&parsed)
	if err != nil {
		c.logVerbose("Failed to decode register response: %v", err)
		return fmt.Errorf("failed to decode register response: %v", err)
	}

	c.clientID = parsed.ClientID
	return nil
}

func (c *APIClient) Heartbeat() error {
	_, err := c.post("/api/learn-clients/heartbeat", nil, true)
	return err
}

var ErrNoJob = errors.New("no job available")

func (c *APIClient) GetJob() (models.Job, error) {
	response, err := c.get("/api/learn-clients/job", true)
	if err != nil {
		c.logVerbose("Failed to get job: %v", err)
		return models.Job{}, fmt.Errorf("failed to get job: %v", err)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		c.logVerbose("Failed to read job response body: %v", err)
		return models.Job{}, fmt.Errorf("failed to read job response body: %v", err)
	}

	if string(body) == "null" {
		return models.Job{}, ErrNoJob
	}

	var job models.Job
	err = json.Unmarshal(body, &job)
	if err != nil {

		c.logVerbose("Failed to unmarshal job response: %v", err)
		return models.Job{}, fmt.Errorf("failed to unmarshal job response: %v", err)
	}

	return job, nil
}

func (c *APIClient) SubmitJobResult(payload models.EvaluationsPayload) error {
	_, err := c.post("/api/positions/evaluations", payload, true)
	if err != nil {
		c.logVerbose("Failed to submit job result: %v", err)
		return fmt.Errorf("failed to submit job result: %v", err)
	}

	return nil
}

func (c *APIClient) LookupPositions(positions []models.NormalizedPosition) ([]models.Evaluation, error) {
	payload := models.LookupPositionsPayload{
		Positions: positions,
	}

	response, err := c.post("/api/positions/lookup", payload, true)
	if err != nil {
		c.logVerbose("Failed to lookup positions: %v", err)
		return nil, fmt.Errorf("failed to lookup positions: %v", err)
	}

	var evaluations []models.Evaluation
	err = json.NewDecoder(response.Body).Decode(&evaluations)
	if err != nil {
		c.logVerbose("Failed to decode lookup positions response: %v", err)
		return nil, fmt.Errorf("failed to decode lookup positions response: %v", err)
	}

	return evaluations, nil
}

func (c *APIClient) SaveLearnedEvaluations(evaluations []models.Evaluation) error {
	if len(evaluations) == 0 {
		c.logVerbose("No evaluations to save")
		return nil
	}

	payload := models.EvaluationsPayload{
		Evaluations: evaluations,
	}

	_, err := c.post("/api/positions/evaluations", payload, true)
	if err != nil {
		c.logVerbose("Failed to save learned evaluations: %v", err)
		return fmt.Errorf("failed to save learned evaluations: %v", err)
	}

	return nil
}
