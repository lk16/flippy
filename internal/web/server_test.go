package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func testServer(t *testing.T) *Server {
	t.Helper()

	s, err := NewServer()
	require.NoError(t, err)
	return s
}

func doGet(t *testing.T, s *Server, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestNewServer_ParsesAllPageTemplates(t *testing.T) {
	testServer(t)
}

func TestHandler_Root_RedirectsToGame(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/")
	require.Equal(t, http.StatusFound, w.Code)
	require.Equal(t, "/game", w.Header().Get("Location"))
}

func TestHandler_Game_RendersBoardAndActiveNavLink(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/game")
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	require.Contains(t, body, `id="board"`)
	require.Contains(t, body, `/static/board.js`)
	require.Contains(t, body, `href="/game" class="active"`)
}

func TestHandler_Stats_RendersTable(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/stats")
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	require.Contains(t, body, `id="stats-table"`)
	require.Contains(t, body, `/static/stats.js`)
	require.Contains(t, body, `href="/stats" class="active"`)
}

func TestHandler_Clients_RendersWorkerTable(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/clients")
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	require.Contains(t, body, `id="worker-table-body"`)
	require.Contains(t, body, `/static/clients.js`)
	require.Contains(t, body, `href="/clients" class="active"`)
}

func TestHandler_EveryPage_LinksToEveryOtherPage(t *testing.T) {
	s := testServer(t)

	for _, p := range pages {
		body := doGet(t, s, p.Path).Body.String()
		for _, other := range pages {
			require.Contains(t, body, `href="`+other.Path+`"`, "page %s missing nav link to %s", p.ID, other.ID)
		}
	}
}

func TestHandler_StaticAsset_Serves(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/static/base.css")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "--bg")
}

func TestHandler_StaticAsset_NotFound(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/static/does-not-exist.css")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_UnknownPage_NotFound(t *testing.T) {
	s := testServer(t)
	w := doGet(t, s, "/does-not-exist")
	require.Equal(t, http.StatusNotFound, w.Code)
}
