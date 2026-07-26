package api

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRedisClient_InvalidURL(t *testing.T) {
	_, err := NewRedisClient(context.Background(), "not-a-valid-url")
	require.Error(t, err)
}

func TestNewRedisClient_Success(t *testing.T) {
	url := os.Getenv("FLIPPY_REDIS_URL")
	if url == "" {
		t.Skip("FLIPPY_REDIS_URL not set; skipping test requiring redis")
	}

	client, err := NewRedisClient(context.Background(), url)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
}
