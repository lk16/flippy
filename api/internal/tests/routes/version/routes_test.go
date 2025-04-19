package tests

import (
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/assert"
)

func TestVersionEndpoint(t *testing.T) {
	resp, err := http.Get(tests.BaseURL + "/version")

	// TODO check actual version

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
