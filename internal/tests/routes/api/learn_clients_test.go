package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestGetClientsNoAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients", nil)
	require.NoError(t, err)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetClientsOkNoClients(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var stats models.StatsResponse
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	require.Empty(t, stats.ClientStats)
	require.Equal(t, 0, stats.ActiveClients)
}

func TestGetClientsOkWithClients(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	require.NoError(t, err)

	registerReq, err := http.NewRequest(
		http.MethodPost,
		"/api/learn-clients/register",
		bytes.NewBuffer(registerPayloadBytes),
	)
	require.NoError(t, err)

	registerReq.Header.Set("X-Token", tests.TestToken)
	registerReq.Header.Set("Content-Type", "application/json")

	app, _ := internal.SetupApp()
	registerResp, err := app.Test(registerReq)
	require.NoError(t, err)

	defer registerResp.Body.Close()

	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registered models.RegisterResponse
	err = json.NewDecoder(registerResp.Body).Decode(&registered)
	require.NoError(t, err)

	require.NotEmpty(t, registered.ClientID)

	clientID := registered.ClientID

	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var stats models.StatsResponse
	err = json.NewDecoder(resp.Body).Decode(&stats)
	require.NoError(t, err)

	require.Equal(t, 1, stats.ActiveClients)
	require.Len(t, stats.ClientStats, 1)

	require.Equal(t, clientID, stats.ClientStats[0].ID)
	require.Equal(t, "test-hostname", stats.ClientStats[0].Hostname)
	require.Equal(t, "test-git-commit", stats.ClientStats[0].GitCommit)
	require.Equal(t, 0, stats.ClientStats[0].PositionsComputed)

	// Delete the client from Redis
	services, err := services.InitServices(config.LoadServerConfig())
	require.NoError(t, err)
	deleted, err := services.Redis.Del(t.Context(), repository.ClientsKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
}

func TestRegisterClientNoAuth(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPost,
		"/api/learn-clients/register",
		bytes.NewBuffer(registerPayloadBytes),
	)
	require.NoError(t, err)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatNoAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/api/learn-clients/heartbeat", nil)
	require.NoError(t, err)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatNoClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/api/learn-clients/heartbeat", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatNoClientUnknownClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/api/learn-clients/heartbeat", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)
	req.Header.Set("X-Client-Id", "unknown-client-id")

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHeartbeatOk(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	require.NoError(t, err)

	registerReq, err := http.NewRequest(
		http.MethodPost,
		"/api/learn-clients/register",
		bytes.NewBuffer(registerPayloadBytes),
	)
	require.NoError(t, err)

	registerReq.Header.Set("X-Token", tests.TestToken)
	registerReq.Header.Set("Content-Type", "application/json")

	app, _ := internal.SetupApp()

	registerResp, err := app.Test(registerReq)
	require.NoError(t, err)

	defer registerResp.Body.Close()

	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registered models.RegisterResponse
	err = json.NewDecoder(registerResp.Body).Decode(&registered)
	require.NoError(t, err)

	clientID := registered.ClientID

	heartbeatReq, err := http.NewRequest(http.MethodPost, "/api/learn-clients/heartbeat", nil)
	require.NoError(t, err)

	heartbeatReq.Header.Set("X-Token", tests.TestToken)
	heartbeatReq.Header.Set("X-Client-Id", clientID)

	resp, err := app.Test(heartbeatReq)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete the client from Redis
	services, err := services.InitServices(config.LoadServerConfig())
	require.NoError(t, err)
	deleted, err := services.Redis.Del(t.Context(), repository.ClientsKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
}

func TestGetJobNoAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients/job", nil)
	require.NoError(t, err)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobNoClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients/job", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobNoClientUnknownClientID(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients/job", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)
	req.Header.Set("X-Client-Id", "unknown-client-id")

	app, _ := internal.SetupApp()

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobOk(t *testing.T) {
	registerPayload := models.RegisterRequest{
		Hostname:  "test-hostname",
		GitCommit: "test-git-commit",
	}
	registerPayloadBytes, err := json.Marshal(registerPayload)
	require.NoError(t, err)

	registerReq, err := http.NewRequest(
		http.MethodPost,
		"/api/learn-clients/register",
		bytes.NewBuffer(registerPayloadBytes),
	)
	require.NoError(t, err)

	registerReq.Header.Set("X-Token", tests.TestToken)
	registerReq.Header.Set("Content-Type", "application/json")

	app, _ := internal.SetupApp()

	registerResp, err := app.Test(registerReq)
	require.NoError(t, err)

	defer registerResp.Body.Close()

	require.Equal(t, http.StatusOK, registerResp.StatusCode)

	var registered models.RegisterResponse
	err = json.NewDecoder(registerResp.Body).Decode(&registered)
	require.NoError(t, err)

	clientID := registered.ClientID

	req, err := http.NewRequest(http.MethodGet, "/api/learn-clients/job", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)
	req.Header.Set("X-Client-Id", clientID)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var job models.Job
	err = json.NewDecoder(resp.Body).Decode(&job)
	require.NoError(t, err)

	// Delete the client from Redis
	services, err := services.InitServices(config.LoadServerConfig())
	require.NoError(t, err)
	deleted, err := services.Redis.Del(t.Context(), repository.ClientsKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
}
