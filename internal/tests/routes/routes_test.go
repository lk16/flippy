package routes_test

import (
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal"
	"github.com/stretchr/testify/require"
)

func TestRootEndpoint(t *testing.T) {
	app, _ := internal.SetupApp()

	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/book", resp.Header.Get("Location"))
}
