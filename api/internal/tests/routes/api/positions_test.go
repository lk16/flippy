package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/assert"
)

func newNormalizedPositionMust(player, opponent uint64) models.NormalizedPosition {
	normalizedPosition, err := models.NewNormalizedPositionFromUint64s(player, opponent)
	if err != nil {
		panic(err)
	}
	return normalizedPosition
}

func TestLookupPositions(t *testing.T) {
	baseURL := tests.BaseURL

	tests := []struct {
		name           string
		positions      []models.NormalizedPosition
		token          string
		wantStatusCode int
		wantCount      int
	}{
		{
			name:           "no auth",
			positions:      []models.NormalizedPosition{newNormalizedPositionMust(0, 0)},
			token:          "",
			wantStatusCode: http.StatusUnauthorized,
			wantCount:      0,
		},
		{
			name:           "invalid payload",
			positions:      nil,
			token:          tests.TestToken,
			wantStatusCode: http.StatusBadRequest,
			wantCount:      0,
		},
		{
			name:           "no results",
			positions:      []models.NormalizedPosition{newNormalizedPositionMust(0, 0)},
			token:          tests.TestToken,
			wantStatusCode: http.StatusOK,
			wantCount:      0,
		},
		{
			name: "with results",
			positions: []models.NormalizedPosition{
				newNormalizedPositionMust(0x0000043630300000, 0x000838080E080000),
				newNormalizedPositionMust(0x00002733373F1C08, 0x243C180C08000020),
				newNormalizedPositionMust(0x00000102380C0400, 0x1E3C3E3C07030000),
				newNormalizedPositionMust(0x0000042050203400, 0x0000B8DEAC580800),
			},
			token:          tests.TestToken,
			wantStatusCode: http.StatusOK,
			wantCount:      4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload bytes.Buffer
			if tt.positions != nil {
				json.NewEncoder(&payload).Encode(models.LookupPositionsPayload{
					Positions: tt.positions,
				})
			}

			req, err := http.NewRequest(http.MethodPost, baseURL+"/api/positions/lookup", &payload)
			assert.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				req.Header.Set("x-token", tt.token)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			assert.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.wantStatusCode, resp.StatusCode)

			if tt.wantStatusCode == http.StatusOK {
				var response []models.Evaluation
				err = json.NewDecoder(resp.Body).Decode(&response)
				assert.NoError(t, err)

				assert.Equal(t, tt.wantCount, len(response))

				if tt.wantCount > 0 {
					for i, position := range tt.positions {
						assert.Equal(t, position.String(), response[i].Position.String())
					}
				}
			}
		})
	}
}

func TestPositionStatsNoAuth(t *testing.T) {
	baseURL := tests.BaseURL

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/positions/stats", nil)
	assert.NoError(t, err)

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestPositionStatsOk(t *testing.T) {
	baseURL := tests.BaseURL

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/positions/stats", nil)
	assert.NoError(t, err)

	req.Header.Set("x-token", tests.TestToken)

	// Run the request twice
	// The first time it will build the stats and store them in Redis
	// The second time it will read from Redis
	// Both responses should be the same
	for i := 0; i < 2; i++ {
		client := &http.Client{}
		resp, err := client.Do(req)
		assert.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response []models.BookStats
		err = json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)

		assert.Equal(t, 5, len(response))
	}
}

// TODO add tests for submit evaluations
