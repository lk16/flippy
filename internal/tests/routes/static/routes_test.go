package static_test

import (
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal"
	"github.com/stretchr/testify/require"
)

func TestStaticFiles(t *testing.T) {
	files := []string{
		"book.css",
		"book.html",
		"book.js",
		"clients.css",
		"clients.html",
		"clients.js",
	}

	app, _ := internal.SetupApp()

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/static/"+file, nil)
			require.NoError(t, err)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
