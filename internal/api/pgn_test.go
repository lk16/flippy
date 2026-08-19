package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// doRequestRaw sends a request with a raw byte body and the given Content-Type.
func doRequestRaw(t *testing.T, s *Server, method, target string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

// validPGN is a real Othello game (FlyOrDie 2025-01-30) with minimal metadata tags.
// ParsePGNLenient ignores metadata, so only the move sequence matters for validity.
const validPGN = `[Event "Test"]
[White "Bob"]
[Black "Alice"]
[Result "0-1"]

1. e6 f4 2. e3 d6 3. c5 f3 4. c6 c3 5. c4 d3 6. c2 b3 7. d2 b5 8. f5 b4 9. g5 e2
10. f2 f1 11. b6 c1 12. f6 g3 13. h4 h3 14. a3 a4 15. a5 g4 16. a2 h5 17. g2 g6
18. e1 d1 19. h6 h1 20. b2 e7 21. g1 h2 22. f8 d8 23. f7 d7 24. g7 c7 25. e8 c8
26. b8 h8 27. h7 g8 0-1`

func TestHandlePGN_ValidGame(t *testing.T) {
	s := testServer(t)

	req := doRequestRaw(t, s, http.MethodPost, "/api/pgn", []byte(validPGN), "text/plain")
	require.Equal(t, http.StatusOK, req.Code)

	var resp pgnResponse
	require.NoError(t, json.Unmarshal(req.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Boards)
	// A full game has 61 boards (positions 0–60).
	require.GreaterOrEqual(t, len(resp.Boards), 2)

	// Every entry must be a parseable board string.
	for _, b := range resp.Boards {
		require.Len(t, b, othello.BoardStringLength, "board string length")
		_, err := othello.ParseBoard(b)
		require.NoError(t, err)
	}
}

func TestHandlePGN_InvalidPGN(t *testing.T) {
	s := testServer(t)
	req := doRequestRaw(t, s, http.MethodPost, "/api/pgn", []byte("this is not pgn at all @@@@"), "text/plain")
	require.Equal(t, http.StatusBadRequest, req.Code)
}

func TestHandlePGN_EmptyBody(t *testing.T) {
	s := testServer(t)
	req := doRequestRaw(t, s, http.MethodPost, "/api/pgn", []byte(""), "text/plain")
	require.Equal(t, http.StatusBadRequest, req.Code)
}

func TestHandlePGN_MultiGameUsesFirst(t *testing.T) {
	s := testServer(t)

	// Two games concatenated; only the first should be parsed.
	twoGames := validPGN + "\n\n" + validPGN
	req := doRequestRaw(t, s, http.MethodPost, "/api/pgn", []byte(twoGames), "text/plain")
	require.Equal(t, http.StatusOK, req.Code)

	var resp pgnResponse
	require.NoError(t, json.Unmarshal(req.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Boards)
}
