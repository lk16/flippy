package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// testWebSocket dials s over an httptest.Server and returns the connection,
// closed automatically when the test ends.
func testWebSocket(t *testing.T, s *Server) *websocket.Conn {
	t.Helper()

	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"

	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn
}

func TestHandleWebSocket_EvaluationRequest_ReturnsFoundEvaluations(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: 20, Depth: 20, Confidence: 98, Score: 3}))

	conn := testWebSocket(t, s)

	unknownBoard := testBoard(t, 13)
	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board.String(), unknownBoard.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 1, resp.ID)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	evaluations, ok := data["evaluations"].([]any)
	require.True(t, ok)
	require.Len(t, evaluations, 1)

	entry, ok := evaluations[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, board.String(), entry["board"])
	require.Equal(t, float64(20), entry["level"])
	require.Equal(t, evaluationSourceEdax, entry["source"])
}

func TestHandleWebSocket_EvaluationRequest_FallsBackToMinimaxCache(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board11 := testBoard(t, book.LeafDiscs-1)
	children := board11.Board().Children()
	require.NotEmpty(t, children)

	normalizedChildren := make([]othello.NormalizedBoard, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, s.repo.AddBoards(ctx, normalizedChildren))
	for _, child := range normalizedChildren {
		require.NoError(t, s.repo.SaveEvaluation(ctx, child, db.Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 1}))
	}
	require.NoError(t, s.cache.Rebuild(ctx))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 7, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board11.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	data := resp.Data.(map[string]any)
	evaluations := data["evaluations"].([]any)
	require.Len(t, evaluations, 1)

	entry := evaluations[0].(map[string]any)
	require.Equal(t, board11.String(), entry["board"])
	require.Equal(t, evaluationSourceMinimax, entry["source"])
}

// TestHandleWebSocket_EvaluationRequest_OmitsUnlearnedBoards covers a board
// that has a row but hasn't been learned yet (still the zero-valued
// Evaluation): it must be omitted from the response just like a board with
// no row at all, not returned as a real score of 0.
func TestHandleWebSocket_EvaluationRequest_OmitsUnlearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	data := resp.Data.(map[string]any)
	require.Nil(t, data["evaluations"])
}

func TestHandleWebSocket_EvaluationRequest_EmptyWhenNothingFound(t *testing.T) {
	s := testServer(t)
	conn := testWebSocket(t, s)

	board := testBoard(t, 12)
	require.NoError(t, wsjson.Write(context.Background(), conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(context.Background(), conn, &resp))

	data := resp.Data.(map[string]any)
	require.Nil(t, data["evaluations"])
}

func TestHandleWebSocket_MalformedBoardIsSkippedNotFatal(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: 20, Depth: 20, Confidence: 98, Score: 3}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{"garbage", board.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	data := resp.Data.(map[string]any)
	evaluations := data["evaluations"].([]any)
	require.Len(t, evaluations, 1)
}

func TestHandleWebSocket_UnknownEventIgnoredConnectionStaysOpen(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{ID: 1, Event: "unknown_event"}))
	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 2, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 2, resp.ID)
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
