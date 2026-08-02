package db

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPool_InvalidURL(t *testing.T) {
	_, err := NewPool(context.Background(), "not-a-valid-url")
	require.Error(t, err)
}

func TestNewPool_Success(t *testing.T) {
	url := os.Getenv("FLIPPY_POSTGRES_URL")
	if url == "" {
		t.Skip("FLIPPY_POSTGRES_URL not set; skipping test requiring Postgres")
	}

	pool, err := NewPool(context.Background(), url)
	require.NoError(t, err)
	defer pool.Close()
}
