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

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: 20, Score: 3}))

	conn := testWebSocket(t, s)

	unknownBoard := testPosition(t, 13)
	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String(), unknownBoard.String()}}),
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
	require.Equal(t, position.String(), entry["board"])
	require.Equal(t, float64(20), entry["level"])
	require.Equal(t, evaluationSourceEdax, entry["source"])
}

func TestHandleWebSocket_EvaluationRequest_FallsBackToMinimaxCache(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position11 := testPosition(t, book.LeafDiscs-1)
	children := position11.Position().Children()
	require.NotEmpty(t, children)

	normalizedChildren := make([]othello.NormalizedPosition, len(children))
	for i, child := range children {
		normalizedChildren[i] = child.Normalize()
	}
	require.NoError(t, s.repo.AddPositions(ctx, normalizedChildren))
	for _, child := range normalizedChildren {
		require.NoError(t, s.repo.SaveEvaluation(ctx, child, db.Evaluation{Level: 20, Score: 1}))
	}
	require.NoError(t, s.cache.Rebuild(ctx))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 7, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position11.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	data := resp.Data.(map[string]any)
	evaluations := data["evaluations"].([]any)
	require.Len(t, evaluations, 1)

	entry := evaluations[0].(map[string]any)
	require.Equal(t, position11.String(), entry["board"])
	require.Equal(t, evaluationSourceMinimax, entry["source"])
}

// A position with a row but no evaluation yet must be omitted, not returned as a real score of 0.
func TestHandleWebSocket_EvaluationRequest_OmitsUnlearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	data := resp.Data.(map[string]any)
	require.Nil(t, data["evaluations"])
}

func TestHandleWebSocket_EvaluationRequest_EmptyWhenNothingFound(t *testing.T) {
	s := testServer(t)
	conn := testWebSocket(t, s)

	position := testPosition(t, 12)
	require.NoError(t, wsjson.Write(context.Background(), conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(context.Background(), conn, &resp))

	data := resp.Data.(map[string]any)
	require.Nil(t, data["evaluations"])
}

func TestHandleWebSocket_MalformedBoardIsSkippedNotFatal(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: 20, Score: 3}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{"garbage", position.String()}}),
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

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{ID: 1, Event: "unknown_event"}))
	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 2, Event: "evaluation_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String()}}),
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

// TestHandleWebSocket_AnalyzeRequest_EnqueuesMissingBoards verifies that positions without an
// existing evaluation are placed on the priority queue, while already-resolved positions are not.
func TestHandleWebSocket_AnalyzeRequest_EnqueuesMissingBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// position1 is already evaluated (DB).
	position1 := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position1}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position1, db.Evaluation{Level: 20, Score: 4}))

	// position2 has no evaluation.
	position2 := testPosition(t, 13)

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position1.String(), position2.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 1, resp.ID)

	// position1 should already be in the response (it's resolved); position2 is not yet.
	data := resp.Data.(map[string]any)
	evaluations, _ := data["evaluations"].([]any)
	require.Len(t, evaluations, 1)
	entry := evaluations[0].(map[string]any)
	require.Equal(t, position1.String(), entry["board"])

	// position2 (normalized) should be in the priority queue.
	pending := drainPriority(t, s)
	pendingBoards := make([]string, len(pending))
	for i, e := range pending {
		pendingBoards[i] = e.Position
	}
	require.Contains(t, pendingBoards, position2.String())
}

// Analyzing a forced-pass position must enqueue the post-pass position instead: edax cannot search a
// position with no legal move.
func TestHandleWebSocket_AnalyzeRequest_ForcedPassEnqueuesPostPassBoard(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	passPosition := testPassRequiredPosition(t)
	passed, err := passPosition.DoMove(othello.PassMove)
	require.NoError(t, err)
	postPassNormalized := passed.Normalize()

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{passPosition.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 1, resp.ID)

	pending := drainPriority(t, s)
	pendingBoards := make([]string, len(pending))
	for i, e := range pending {
		pendingBoards[i] = e.Position
	}

	// The post-pass position is enqueued; the un-searchable pass position is not.
	require.Contains(t, pendingBoards, postPassNormalized.String())
	require.NotContains(t, pendingBoards, passPosition.Normalize().String())
}

// TestHandleWebSocket_AnalyzeRequest_SameSearchNotEnqueued verifies that a deeper level asking for
// a search the position already got is answered from the DB instead of queued: at 30 discs levels 28
// and 29 both mean a 34-ply search at 73%.
func TestHandleWebSocket_AnalyzeRequest_SameSearchNotEnqueued(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 30)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: 28, Score: 7}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String()}, Level: 29}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	// The stored evaluation comes straight back...
	data := resp.Data.(map[string]any)
	evaluations := data["evaluations"].([]any)
	require.Len(t, evaluations, 1)
	entry := evaluations[0].(map[string]any)
	require.Equal(t, float64(7), entry["score"])
	require.Equal(t, float64(34), entry["depth"])
	require.Equal(t, float64(73), entry["confidence"])

	// ... and no job is queued to compute it again.
	pending := drainPriority(t, s)
	require.Empty(t, pending)
}

// TestHandleWebSocket_AnalyzeRequest_FinalResultNotEnqueued verifies that a position already searched
// to the end of the game is never re-queued, however deep a level is requested. At 44 discs every
// level from 10 up solves the 20 remaining empties outright.
func TestHandleWebSocket_AnalyzeRequest_FinalResultNotEnqueued(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Past MaxSavableDiscs, so the ephemeral analysis cache is where its evaluation lives.
	position := testPosition(t, 44)
	s.setAnalysisResult(ctx, position.String(), evaluationResponse{
		Level: 10, Depth: 20, Confidence: 100, Score: 6, Source: evaluationSourceEdax,
	})

	s.handleAnalyzeRequest(ctx, []string{position.String()}, 28, "")

	pending := drainPriority(t, s)
	require.Empty(t, pending)
}

// TestSearchAddsNothing covers when a requested level describes work the stored evaluation already
// did, and when it does not.
func TestSearchAddsNothing(t *testing.T) {
	tests := []struct {
		name      string
		eval      evaluationResponse
		discCount int
		level     int
		want      bool
	}{
		{
			name:      "same depth and confidence at a deeper level",
			eval:      evaluationResponse{Level: 28, Depth: 34, Confidence: 73},
			discCount: 30,
			level:     29,
			want:      true,
		},
		{
			name:      "deeper level buys more confidence",
			eval:      evaluationResponse{Level: 28, Depth: 34, Confidence: 73},
			discCount: 30,
			level:     32,
			want:      false,
		},
		{
			name:      "stored result already ran the game out",
			eval:      evaluationResponse{Level: 10, Depth: 20, Confidence: 100},
			discCount: 44,
			level:     28,
			want:      true,
		},
		{
			name:      "midgame search that a deeper level extends",
			eval:      evaluationResponse{Level: 10, Depth: 10, Confidence: 100},
			discCount: 12,
			level:     12,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, searchAddsNothing(tt.eval, tt.discCount, tt.level))
		})
	}
}

// TestHandleWebSocket_AnalyzeRequest_GameOverNotEnqueued verifies that a game-over position (neither
// player can move) is not enqueued for analysis, since its score is final.
func TestHandleWebSocket_AnalyzeRequest_GameOverNotEnqueued(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// A full position with only one color: no moves for either player → game over.
	gameOver, err := othello.NewPosition(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)
	require.False(t, gameOver.HasMoves())

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 1, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{gameOver.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))

	pending := drainPriority(t, s)
	require.Empty(t, pending)
}

// TestHandleWebSocket_AnalyzeRequest_NoDuplicatesInQueue verifies that repeated analyze_request
// calls for the same position only enqueue it once.
func TestHandleWebSocket_AnalyzeRequest_NoDuplicatesInQueue(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 13)
	conn := testWebSocket(t, s)

	send := func(id int) {
		require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
			ID: id, Event: "analyze_request",
			Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String()}}),
		}))
		var resp wsOutgoing
		require.NoError(t, wsjson.Read(ctx, conn, &resp))
	}

	send(1)
	send(2) // same position again

	// Only one entry must be in the queue.
	dequeued := drainPriority(t, s)
	count := 0
	for _, entry := range dequeued {
		if entry.Position == position.String() {
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

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{Level: 20, Score: 3}))

	conn := testWebSocket(t, s)

	require.NoError(t, wsjson.Write(ctx, conn, wsIncoming{
		ID: 5, Event: "analyze_request",
		Data: mustMarshal(t, wsEvaluationRequest{Positions: []string{position.String()}}),
	}))

	var resp wsOutgoing
	require.NoError(t, wsjson.Read(ctx, conn, &resp))
	require.Equal(t, 5, resp.ID)

	data := resp.Data.(map[string]any)
	_, hasEvaluations := data["evaluations"]
	require.True(t, hasEvaluations, "response must have 'evaluations' key matching evaluation_request shape")
}
