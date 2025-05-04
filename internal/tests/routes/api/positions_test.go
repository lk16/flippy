package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestLookupPositions(t *testing.T) {
	baseURL := tests.BaseURL

	tests := []struct {
		name           string
		nPositions     []models.NormalizedPosition
		token          string
		wantStatusCode int
		wantCount      int
	}{
		{
			name:           "no auth",
			nPositions:     []models.NormalizedPosition{models.NewNormalizedPositionMust(0, 0)},
			token:          "",
			wantStatusCode: http.StatusUnauthorized,
			wantCount:      0,
		},
		{
			name:           "invalid payload",
			nPositions:     nil,
			token:          tests.TestToken,
			wantStatusCode: http.StatusBadRequest,
			wantCount:      0,
		},
		{
			name:           "no results",
			nPositions:     []models.NormalizedPosition{models.NewNormalizedPositionMust(0, 0)},
			token:          tests.TestToken,
			wantStatusCode: http.StatusOK,
			wantCount:      0,
		},
		{
			name: "with results",
			nPositions: []models.NormalizedPosition{
				models.NewNormalizedPositionMust(0x0000043630300000, 0x000838080E080000),
				models.NewNormalizedPositionMust(0x00002733373F1C08, 0x243C180C08000020),
				models.NewNormalizedPositionMust(0x00000102380C0400, 0x1E3C3E3C07030000),
				models.NewNormalizedPositionMust(0x0000042050203400, 0x0000B8DEAC580800),
			},
			token:          tests.TestToken,
			wantStatusCode: http.StatusOK,
			wantCount:      4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload bytes.Buffer
			if tt.nPositions != nil {
				json.NewEncoder(&payload).Encode(models.LookupPositionsPayload{
					Positions: tt.nPositions,
				})
			}

			req, err := http.NewRequest(http.MethodPost, baseURL+"/api/positions/lookup", &payload)
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				req.Header.Set("X-Token", tt.token)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			require.Equal(t, tt.wantStatusCode, resp.StatusCode)

			if tt.wantStatusCode == http.StatusOK {
				var evaluation []models.Evaluation
				err = json.NewDecoder(resp.Body).Decode(&evaluation)
				require.NoError(t, err)

				require.Len(t, evaluation, tt.wantCount)

				if tt.wantCount > 0 {
					for i, position := range tt.nPositions {
						require.Equal(t, position.String(), evaluation[i].Position.String())
					}
				}
			}
		})
	}
}

func TestPositionStatsNoAuth(t *testing.T) {
	baseURL := tests.BaseURL

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/positions/stats", nil)
	require.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestPositionStatsOk(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, tests.BaseURL+"/api/positions/stats", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)

	// Run the request twice
	// The first time it will build the stats and store them in Redis
	// The second time it will read from Redis
	// Both responses should be the same
	for range 2 {
		client := &http.Client{}
		var resp *http.Response
		resp, err = client.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var stats []models.BookStats
		err = json.NewDecoder(resp.Body).Decode(&stats)
		require.NoError(t, err)

		require.Len(t, stats, 5)
	}
}

func TestSubmitEvaluationsNoAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/positions/evaluations", nil)
	require.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSubmitEvaluationsInvalidPayload(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/positions/evaluations", nil)
	require.NoError(t, err)

	req.Header.Set("X-Token", tests.TestToken)

	client := &http.Client{}

	// No payload is an invalid payload, it's not even a valid JSON object
	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSubmitEvaluationsValidationError(t *testing.T) {
	payload := models.EvaluationsPayload{
		Evaluations: []models.Evaluation{
			{
				Position:   models.NewNormalizedPositionMust(0x0, 0x0),
				Level:      0,
				Depth:      0,
				Confidence: 0,
				Score:      0,
				BestMoves:  models.BestMoves{},
			},
		},
	}

	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/positions/evaluations", &buffer)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Token", tests.TestToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSubmitEvaluationsOk(t *testing.T) {
	pos := models.NewPositionStart()

	nPos := models.NewNormalizedPositionMust(pos.Player(), pos.Opponent())

	payload := models.EvaluationsPayload{
		Evaluations: []models.Evaluation{
			{
				Position:   nPos,
				Level:      50,
				Depth:      30,
				Confidence: 73,
				Score:      0,
				BestMoves:  models.BestMoves{19, 18},
			},
		},
	}

	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, tests.BaseURL+"/api/positions/evaluations", &buffer)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Token", tests.TestToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	services, err := services.InitServices(config.LoadServerConfig())
	require.NoError(t, err)

	// Check if the evaluation was stored in the database
	positionRepo := repository.NewEvaluationRepositoryFromServices(services)
	foundPositions, err := positionRepo.LookupPositions(t.Context(), []models.NormalizedPosition{nPos})
	require.NoError(t, err)
	require.Len(t, foundPositions, 1)
	require.Equal(t, payload.Evaluations[0], foundPositions[0])

	// Cleanup inserted item from database
	postgresConn := services.Postgres
	result, err := postgresConn.Exec("DELETE FROM edax WHERE position = $1", nPos.Bytes())
	require.NoError(t, err)

	// Ensure that exactly one row was deleted
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}
