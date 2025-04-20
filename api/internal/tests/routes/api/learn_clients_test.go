package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/assert"
)

func TestGetClientsNoAuth(t *testing.T) {
	baseURL := tests.BaseURL

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/learn-clients", nil)
	assert.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetClientsOkNoClients(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/learn-clients", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response models.StatsResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)

	assert.Equal(t, 0, len(response.ClientStats))
	assert.Equal(t, 0, response.ActiveClients)
}

func TestGetClientsOkWithClients(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	assert.NoError(t, err)

	registerReq, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/register", bytes.NewBuffer(registerPayloadBytes))
	assert.NoError(t, err)

	registerReq.Header.Set("x-token", tests.TestToken)
	registerReq.Header.Set("Content-Type", "application/json")

	registerClient := &http.Client{}
	registerResp, err := registerClient.Do(registerReq)
	assert.NoError(t, err)

	defer registerResp.Body.Close()

	assert.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registerResponse models.RegisterResponse
	err = json.NewDecoder(registerResp.Body).Decode(&registerResponse)
	assert.NoError(t, err)

	assert.NotEmpty(t, registerResponse.ClientID)

	clientID := registerResponse.ClientID

	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/learn-clients", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response models.StatsResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err)

	assert.Equal(t, 1, response.ActiveClients)
	assert.Equal(t, 1, len(response.ClientStats))

	assert.Equal(t, clientID, response.ClientStats[0].ID)
	assert.Equal(t, "test-hostname", response.ClientStats[0].Hostname)
	assert.Equal(t, "test-git-commit", response.ClientStats[0].GitCommit)
	assert.Equal(t, 0, response.ClientStats[0].PositionsComputed)

	// Delete the client from Redis
	services, err := services.InitServices(config.LoadConfig())
	assert.NoError(t, err)
	deleted, err := services.Redis.Del(context.Background(), repository.ClientsKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
}

func TestRegisterClientNoAuth(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/register", bytes.NewBuffer(registerPayloadBytes))
	assert.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	log.Println(string(body))

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatNoAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/heartbeat", nil)
	assert.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatNoClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/heartbeat", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatNoClientUnknownClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/heartbeat", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)
	req.Header.Set("x-client-id", "unknown-client-id")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatOk(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	assert.NoError(t, err)

	registerReq, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/register", bytes.NewBuffer(registerPayloadBytes))
	assert.NoError(t, err)

	registerReq.Header.Set("x-token", tests.TestToken)
	registerReq.Header.Set("Content-Type", "application/json")

	registerClient := &http.Client{}
	registerResp, err := registerClient.Do(registerReq)
	assert.NoError(t, err)

	defer registerResp.Body.Close()

	assert.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registerResponse models.RegisterResponse
	err = json.NewDecoder(registerResp.Body).Decode(&registerResponse)
	assert.NoError(t, err)

	clientID := registerResponse.ClientID

	log.Println(clientID)

	heartbeatReq, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/heartbeat", nil)
	assert.NoError(t, err)

	heartbeatReq.Header.Set("x-token", tests.TestToken)
	heartbeatReq.Header.Set("client-id", clientID)
	client := &http.Client{}
	resp, err := client.Do(heartbeatReq)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete the client from Redis
	services, err := services.InitServices(config.LoadConfig())
	assert.NoError(t, err)
	deleted, err := services.Redis.Del(context.Background(), repository.ClientsKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
}

func TestGetJobNoAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/learn-clients/job", nil)
	assert.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobNoClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/learn-clients/job", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobNoClientUnknownClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/learn-clients/job", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)
	req.Header.Set("client-id", "unknown-client-id")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobOk(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	assert.NoError(t, err)

	registerReq, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/learn-clients/register", bytes.NewBuffer(registerPayloadBytes))
	assert.NoError(t, err)

	registerReq.Header.Set("x-token", tests.TestToken)
	registerReq.Header.Set("Content-Type", "application/json")

	registerClient := &http.Client{}
	registerResp, err := registerClient.Do(registerReq)
	assert.NoError(t, err)

	defer registerResp.Body.Close()

	assert.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registerResponse models.RegisterResponse
	err = json.NewDecoder(registerResp.Body).Decode(&registerResponse)
	assert.NoError(t, err)

	clientID := registerResponse.ClientID

	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/learn-clients/job", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)
	req.Header.Set("client-id", clientID)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var job models.Job
	err = json.NewDecoder(resp.Body).Decode(&job)
	assert.NoError(t, err)

	// Delete the client from Redis
	services, err := services.InitServices(config.LoadConfig())
	assert.NoError(t, err)
	deleted, err := services.Redis.Del(context.Background(), repository.ClientsKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
}
