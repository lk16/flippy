package version_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestVersionEndpoint(t *testing.T) {
	resp, err := http.Get(tests.BaseURL + "/version")

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var version models.VersionResponse
	err = json.NewDecoder(resp.Body).Decode(&version)
	require.NoError(t, err)
}
