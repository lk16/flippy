package book

import (
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	heartbeatInterval = time.Minute
	noJobSleepTime    = 10 * time.Second
	errorSleepTime    = 10 * time.Second
)

type LearnClient struct {
	apiClient *APIClient
}

func NewLearnClient(config *config.LearnClientConfig) (*LearnClient, error) {
	apiClient, err := NewAPIClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create api client: %w", err)
	}

	return &LearnClient{
		apiClient: apiClient,
	}, nil
}

func (c *LearnClient) heartbeatLoop() {
	for {
		time.Sleep(heartbeatInterval)

		err := c.apiClient.Heartbeat()
		if err != nil {
			slog.Error("Failed to send heartbeat", "error", err)
		}
	}
}

func (c *LearnClient) doJobsLoop() {
	jobCount := 0
	totalJobTimeSec := 0.0

	resultChan := make(chan edax.Result)
	edaxManager := edax.NewProcess(resultChan)

	for {
		job, err := c.apiClient.GetJob()
		if err != nil {
			slog.Error("Failed to get learn job", "error", err)
			time.Sleep(noJobSleepTime)
			continue
		}

		discCount := job.Position.CountDiscs()
		slog.Info("Got job", "job_number", jobCount+1, "disc_count", discCount, "learn_level", job.Level)
		for _, line := range job.Position.ASCIIArtLines() {
			slog.Info("Position", "line", line)
		}

		jobResult, err := edaxManager.DoJobSync(job)
		if err != nil {
			log.Printf("failed to do job: %s", err.Error())
			time.Sleep(errorSleepTime)
			continue
		}

		jobCount++
		totalJobTimeSec += jobResult.ComputationTime
		avgJobTimeSeconds := float64(totalJobTimeSec) / float64(jobCount)
		slog.Info("Total jobs", "job_count", jobCount, "avg_job_time", avgJobTimeSeconds)

		payload := models.EvaluationsPayload{
			Evaluations: []models.Evaluation{jobResult.Evaluation},
		}

		err = c.apiClient.SubmitJobResult(payload)
		if err != nil {
			slog.Error("Failed to submit job result", "error", err)
			time.Sleep(errorSleepTime)
			continue
		}
	}
}

func (c *LearnClient) Run() {
	go c.heartbeatLoop()
	c.doJobsLoop()
}
