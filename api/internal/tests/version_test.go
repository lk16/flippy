package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionEndpoint(t *testing.T) {
	resp, err := http.Get(BaseURL + "/version")

	// TODO check actual version

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
