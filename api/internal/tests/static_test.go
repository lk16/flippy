package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
			resp, err := http.Get(BaseURL + "/static/" + file)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
