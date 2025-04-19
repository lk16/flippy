package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootEndpoint(t *testing.T) {
	client := http.DefaultClient

	resp, err := client.Get("http://localhost:3000/version")

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
