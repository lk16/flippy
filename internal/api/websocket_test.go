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

// TestHandleWebSocket_AnalyzeRequest_EnqueuesMissingBoards verifies that boards without an
// existing evaluation are placed on the priority queue, while already-resolved boards are not.
func TestHandleWebSocket_AnalyzeRequest_EnqueuesMissingBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// board1 is already evaluated (DB).
	board1 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board1}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board1, db.Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 4}))

	// board2 has no evaluation.
	board2 := testBoard(t, 13)

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board1.String(), board2.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 1, resp.ID)

	// board1 should already be in the response (it's resolved); board2 is not yet.
	data := resp.Data.(map[string]any)
	evaluations, _ := data["evaluations"].([]any)
	require.Len(t, evaluations, 1)
	entry := evaluations[0].(map[string]any)
	require.Equal(t, board1.String(), entry["board"])

	// board2 (normalized) should be in the priority queue.
	pending, err := s.dequeuePriority(ctx, 10)
	require.NoError(t, err)
	pendingBoards := make([]string, len(pending))
	for i, e := range pending {
		pendingBoards[i] = e.Board
	}
	require.Contains(t, pendingBoards, board2.String())
}

// TestHandleWebSocket_AnalyzeRequest_ForcedPassEnqueuesPostPassBoard verifies that requesting
// analysis of a forced-pass board (no legal move, opponent can move) enqueues the board *after*
// the pass — whose negated evaluation is the pass board's evaluation — rather than the pass board
// itself, which edax cannot search.
func TestHandleWebSocket_AnalyzeRequest_ForcedPassEnqueuesPostPassBoard(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	passBoard := testPassRequiredBoard(t)
	passed, err := passBoard.DoMove(othello.PassMove)
	require.NoError(t, err)
	postPassNormalized := passed.Normalize()

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{passBoard.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 1, resp.ID)

	pending, err := s.dequeuePriority(ctx, 10)
	require.NoError(t, err)
	pendingBoards := make([]string, len(pending))
	for i, e := range pending {
		pendingBoards[i] = e.Board
	}

	// The post-pass board is enqueued; the un-searchable pass board is not.
	require.Contains(t, pendingBoards, postPassNormalized.String())
	require.NotContains(t, pendingBoards, passBoard.Normalize().String())
}

// TestHandleWebSocket_AnalyzeRequest_GameOverNotEnqueued verifies that a game-over board (neither
// player can move) is not enqueued for analysis, since its score is final.
func TestHandleWebSocket_AnalyzeRequest_GameOverNotEnqueued(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// A full board with only one color: no moves for either player → game over.
	gameOver, err := othello.NewBoard(0xFFFFFFFFFFFFFFFF, 0, othello.Black)
	require.NoError(t, err)
	require.False(t, gameOver.HasMoves())

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{gameOver.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	pending, err := s.dequeuePriority(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, pending)
}

// TestHandleWebSocket_AnalyzeRequest_NoDuplicatesInQueue verifies that repeated analyze_request
// calls for the same board only enqueue it once.
func TestHandleWebSocket_AnalyzeRequest_NoDuplicatesInQueue(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 13)
	conn := testWebSocket(t, s)

	send := func(id int) {
		require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
			ID: id, Event: "analyze_request",
			Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board.String()}}),
		}))
		var resp wsOutgoing
		require.NoError(t, wsjson.Read(ctx, conn, &resp))
	}

	send(1)
	send(2) // same board again

	// Only one entry must be in the queue.
	dequeued, err := s.dequeuePriority(ctx, 10)
	require.NoError(t, err)
	count := 0
	for _, entry := range dequeued {
		if entry.Board == board.String() {
			count++
		}
	}
	require.Equal(t, 1, count)
}

// TestHandleWebSocket_AnalyzeRequest_SameShapeAsEvaluationRequest confirms the response uses
// the same wsEvaluationResponse shape as evaluation_request.
func TestHandleWebSocket_AnalyzeRequest_SameShapeAsEvaluationRequest(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: 20, Depth: 20, Confidence: 100, Score: 3}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 5, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Boards: []string{board.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 5, resp.ID)

	data := resp.Data.(map[string]any)
	_, hasEvaluations := data["evaluations"]
	require.True(t, hasEvaluations, "response must have 'evaluations' key matching evaluation_request shape")
}
