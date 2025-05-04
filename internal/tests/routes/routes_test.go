package routes_test

import (
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestRootEndpoint(t *testing.T) {
	// Disable redirects
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(tests.BaseURL)

	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/book", resp.Header.Get("Location"))
}
