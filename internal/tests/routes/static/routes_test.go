package static_test

import (
	"net/http"
	"testing"

	"github.com/lk16/flippy/api/internal/tests"
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

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			resp, err := http.Get(tests.BaseURL + "/static/" + file)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
