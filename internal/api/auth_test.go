package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// doRequestWithToken is doRequest without its automatic valid token: token "" sends no
// Authorization header at all.
func doRequestWithToken(t *testing.T, s *Server, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestRequireWorkerToken(t *testing.T) {
	s := testServer(t)

	workerEndpoints := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/jobs?worker_id=w1"},
		{http.MethodPost, "/api/jobs/result"},
		{http.MethodPost, "/api/jobs/release"},
		{http.MethodPost, "/api/workers/heartbeat"},
		{http.MethodPost, "/api/redis/flush"},
	}

	for _, endpoint := range workerEndpoints {
		t.Run(endpoint.target, func(t *testing.T) {
			w := doRequestWithToken(t, s, endpoint.method, endpoint.target, "")
			require.Equal(t, http.StatusUnauthorized, w.Code)

			w = doRequestWithToken(t, s, endpoint.method, endpoint.target, "wrong-token")
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}

	t.Run("open endpoint needs no token", func(t *testing.T) {
		w := doRequestWithToken(t, s, http.MethodGet, "/api/stats", "")
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestWorkerTokenMatches(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		presented  string
		want       bool
	}{
		{"match", "secret", "secret", true},
		{"mismatch", "secret", "other", false},
		{"empty presented", "secret", "", false},
		{"empty configured fails closed", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, workerTokenMatches(tt.configured, tt.presented))
		})
	}
}
