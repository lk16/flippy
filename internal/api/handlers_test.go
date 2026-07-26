package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

func doRequest(t *testing.T, s *Server, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, target, reader)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestHandleGetJob_MissingWorkerID(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/jobs", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetJob_NoJobsAvailable(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/jobs?worker_id=w1", nil)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandleGetJob_ReturnsJob(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(context.Background(), []othello.NormalizedBoard{board}))

	w := doRequest(t, s, http.MethodGet, "/api/jobs?worker_id=w1", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp jobResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, board.String(), resp.Board)
	require.Equal(t, TargetLevel(12), resp.Level)
}

func TestHandleSubmitJobResult_Success(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	reqBody := jobResultRequest{
		WorkerID: "w1", Board: board.String(), Level: 24, Depth: 24, Confidence: 100, Score: 4,
	}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusOK, w.Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: 24, Depth: 24, Confidence: 100, Score: 4}, eval)

	// Claim must be released: another worker can now claim the same board.
	claimed, err = s.tryClaim(ctx, board.String(), "w2")
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestHandleSubmitJobResult_RebuildsMinimaxCache(t *testing.T) {
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

	_, ok := s.cache.Get(board11.Board())
	require.False(t, ok)

	// Submitting each child's result via the HTTP endpoint (not calling
	// Rebuild directly) must be what makes board11 resolve, once the last
	// leaf it depends on is learned.
	for _, child := range normalizedChildren {
		reqBody := jobResultRequest{
			WorkerID: "w1", Board: child.String(), Level: 24, Depth: 24, Confidence: 100, Score: 1,
		}
		w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
		require.Equal(t, http.StatusOK, w.Code)
	}

	_, ok = s.cache.Get(board11.Board())
	require.True(t, ok)
}

func TestHandleSubmitJobResult_InvalidBody(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/result", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSubmitJobResult_MissingWorkerID(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	reqBody := jobResultRequest{Board: board.String(), Level: 24, Depth: 24, Confidence: 100, Score: 0}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSubmitJobResult_InvalidBoard(t *testing.T) {
	s := testServer(t)
	reqBody := jobResultRequest{WorkerID: "w1", Board: "garbage", Level: 24, Depth: 24, Confidence: 100, Score: 0}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSubmitJobResult_OutOfRangeValues(t *testing.T) {
	board := testBoard(t, 12)

	tests := []struct {
		name string
		req  jobResultRequest
	}{
		{
			name: "score too high",
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Depth: 24, Confidence: 100, Score: 100},
		},
		{
			name: "non-positive level",
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 0, Depth: 24, Confidence: 100, Score: 0},
		},
		{
			name: "negative depth",
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Depth: -1, Confidence: 100, Score: 0},
		},
		{
			name: "depth too high",
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Depth: 61, Confidence: 100, Score: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testServer(t)
			w := doRequest(t, s, http.MethodPost, "/api/jobs/result", tt.req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleSubmitJobResult_BoardNotFound(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	reqBody := jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Depth: 24, Confidence: 100, Score: 0}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetBoard_MissingParam(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/boards", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetBoard_InvalidBoard(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/boards?board=garbage", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetBoard_NotFound(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	target := "/api/boards?board=" + url.QueryEscape(board.String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetBoard_Success(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: 20, Depth: 20, Confidence: 98, Score: 2}))

	target := "/api/boards?board=" + url.QueryEscape(board.Board().String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp evaluationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, evaluationResponse{Level: 20, Depth: 20, Confidence: 98, Score: 2, Source: evaluationSourceEdax}, resp)
}

func TestHandleGetBoard_FallsBackToMinimaxCache(t *testing.T) {
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

	target := "/api/boards?board=" + url.QueryEscape(board11.Board().String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp evaluationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, evaluationSourceMinimax, resp.Source)
	score, ok := s.cache.Get(board11.Board())
	require.True(t, ok)
	require.Equal(t, score, resp.Score)
}

func TestHandleHeartbeat_MissingWorkerID(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/workers/heartbeat", heartbeatRequest{})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleHeartbeat_Success(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/workers/heartbeat", heartbeatRequest{WorkerID: "w1"})
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleStats_Empty(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/stats", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `[]`, w.Body.String())
}

func TestHandleStats_ReturnsCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	w := doRequest(t, s, http.MethodGet, "/api/stats", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var entries []statEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	require.Contains(t, entries, statEntry{DiscCount: 12, Level: 0, Count: 1})
}

func TestHandleListWorkers_Empty(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/workers", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `[]`, w.Body.String())
}

func TestHandleListWorkers_ReturnsActiveWorkers(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "w1", "host-1", "commit-1"))

	w := doRequest(t, s, http.MethodGet, "/api/workers", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var workers []workerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &workers))
	require.Len(t, workers, 1)
	require.Equal(t, "w1", workers[0].ID)
	require.Equal(t, "host-1", workers[0].Hostname)
	require.Equal(t, "commit-1", workers[0].GitCommit)
}
