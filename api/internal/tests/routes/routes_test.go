package tests

import (
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/assert"
)

func TestRootEndpoint(t *testing.T) {
	// Disable redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(tests.BaseURL)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/book", resp.Header.Get("Location"))
}
