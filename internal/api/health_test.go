package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleHealthz(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/healthz", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleReadyz(t *testing.T) {
	t.Run("cache not built yet", func(t *testing.T) {
		s := testServer(t)
		w := doRequest(t, s, http.MethodGet, "/readyz", nil)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("ready", func(t *testing.T) {
		s := testServer(t)
		require.NoError(t, s.cache.Rebuild(context.Background()))

		w := doRequest(t, s, http.MethodGet, "/readyz", nil)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("redis down", func(t *testing.T) {
		s := testServer(t)
		require.NoError(t, s.cache.Rebuild(context.Background()))
		require.NoError(t, s.redis.Close())

		w := doRequest(t, s, http.MethodGet, "/readyz", nil)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestHandleVersion(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/version", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp versionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Commit)
}
