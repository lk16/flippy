package version_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/stretchr/testify/require"
)

func TestVersionEndpoint(t *testing.T) {
	app, _ := internal.SetupApp()

	req, err := http.NewRequest(http.MethodGet, "/version", nil)
	require.NoError(t, err)

	resp, err := app.Test(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var version models.VersionResponse
	err = json.NewDecoder(resp.Body).Decode(&version)
	require.NoError(t, err)
}
