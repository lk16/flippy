package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/routes/version"
	"github.com/stretchr/testify/assert"
)

func TestVersionEndpoint(t *testing.T) {
	resp, err := http.Get(BaseURL + "/version")

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var version version.VersionResponse
	err = json.NewDecoder(resp.Body).Decode(&version)
	assert.NoError(t, err)
}
