package book

import (
	"log"
	"time"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	heartbeatInterval = time.Minute
)

type LearnClient struct {
	apiClient *APIClient
	verbose   bool
}

func NewLearnClient(config *config.LearnClientConfig, verbose bool) *LearnClient {
	return &LearnClient{
		apiClient: NewAPIClient(config, verbose),
		verbose:   verbose,
	}
}

func (c *LearnClient) heartbeatLoop() {
	for {
		time.Sleep(heartbeatInterval)

		err := c.apiClient.Heartbeat()
		if err != nil {
			log.Printf("Failed to send heartbeat: %v", err)
		}
	}
}

func (c *LearnClient) doJobsLoop() {
	jobCount := 0
	totalJobTimeSec := 0.0
	edaxManager := edax.GetEdaxManager(c.verbose)
	for {
		job, err := c.apiClient.GetJob()
		if err != nil {
			log.Printf("Failed to get learn job: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		discCount := job.Position.CountDiscs()
		log.Printf("Got job %d | %d discs | learn level %d:", jobCount+1, discCount, job.Level)
		for _, line := range job.Position.AsciiArtLines() {
			log.Printf("%s", line)
		}

		jobResult, err := edaxManager.DoJob(job)
		if err != nil {
			log.Printf("Failed to do job: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		jobCount++
		totalJobTimeSec += jobResult.ComputationTime
		avgJobTimeSeconds := float64(totalJobTimeSec) / float64(jobCount)
		log.Printf("Total jobs: %d | Average time: %.2f sec", jobCount, avgJobTimeSeconds)

		payload := models.EvaluationsPayload{
			Evaluations: []models.Evaluation{jobResult.Evaluation},
		}

		err = c.apiClient.SubmitJobResult(payload)
		if err != nil {
			log.Printf("Failed to submit job result: %v", err)
			continue
		}
	}
}

func (c *LearnClient) Run() {
	go c.heartbeatLoop()
	c.doJobsLoop()
}
