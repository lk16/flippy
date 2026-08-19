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
	"github.com/lk16/flippy/internal/edax"
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

func TestHandleGetJob_RepeatedRequestsReturnDistinctJobs(t *testing.T) {
	s := testServer(t)
	boards := testDistinctBoards(t, 12, 2)
	require.NoError(t, s.repo.AddBoards(context.Background(), boards))

	w := doRequest(t, s, http.MethodGet, "/api/jobs?worker_id=w1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var first jobResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))

	w = doRequest(t, s, http.MethodGet, "/api/jobs?worker_id=w1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var second jobResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &second))

	require.NotEqual(t, first.Board, second.Board)
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
		WorkerID: "w1", Board: board.String(), Level: TargetLevel(12), Score: 4,
	}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusOK, w.Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: TargetLevel(12), Score: 4}, eval)

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
			WorkerID: "w1", Board: child.String(), Level: TargetLevel(book.LeafDiscs), Score: 1,
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
	reqBody := jobResultRequest{Board: board.String(), Level: 24, Depth: 24, Confidence: 73, Score: 0}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSubmitJobResult_InvalidBoard(t *testing.T) {
	s := testServer(t)
	reqBody := jobResultRequest{WorkerID: "w1", Board: "garbage", Level: 24, Depth: 24, Confidence: 73, Score: 0}
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
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Depth: 24, Confidence: 73, Score: 100},
		},
		{
			name: "non-positive level",
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 0, Depth: 24, Confidence: 100, Score: 0},
		},
		{
			// Would overflow the smallint column and 500 without an upper bound.
			name: "level above smallint-safe max",
			req:  jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 100000, Depth: 24, Confidence: 100, Score: 0},
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

// TestHandleSubmitJobResult_AcceptsMismatchedSearchParams checks that a result whose reported
// depth/confidence disagree with edax's level table is still stored: the mismatch is only worth a
// warning (see checkReportedSearchParams), and the score it came with is a real search result.
func TestHandleSubmitJobResult_AcceptsMismatchedSearchParams(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	// A level-40 search of a 12-disc board is not 30@98%.
	reqBody := jobResultRequest{WorkerID: "w1", Board: board.String(), Level: TargetLevel(12), Depth: 30, Confidence: 98, Score: 4}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusOK, w.Code)

	stored, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: TargetLevel(12), Score: 4}, stored)
}

func TestHandleSubmitJobResult_BoardNotFound(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	reqBody := jobResultRequest{WorkerID: "w1", Board: board.String(), Level: TargetLevel(12), Score: 0}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleSubmitJobResult_NonPriorityBelowTargetNotPersisted covers the API-wide book-quality
// floor: a below-target result is accepted (cached ephemerally, claim released) but never saved,
// even on the non-priority path.
func TestHandleSubmitJobResult_NonPriorityBelowTargetNotPersisted(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	require.Less(t, 24, TargetLevel(12))
	reqBody := jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Score: 4}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/result", reqBody)
	require.Equal(t, http.StatusOK, w.Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval, "row stays unevaluated")

	// The result is still served back from the ephemeral analysis cache.
	cached, ok, err := s.getAnalysisResult(ctx, board.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 4, cached.Score)

	// The claim was released.
	claimed, err = s.tryClaim(ctx, board.String(), "w2")
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestHandleReleaseJob_AllowsAnotherWorkerToClaim(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	reqBody := releaseJobRequest{WorkerID: "w1", Board: board.String()}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/release", reqBody)
	require.Equal(t, http.StatusOK, w.Code)

	claimed, err = s.tryClaim(ctx, board.String(), "w2")
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestHandleReleaseJob_NoActiveClaimIsNoop(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)

	reqBody := releaseJobRequest{WorkerID: "w1", Board: board.String()}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/release", reqBody)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleReleaseJob_DoesNotRevokeAnotherWorkersClaim(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)

	// Simulates w1's claim TTL having expired and w2 winning the re-claim before w1's late release.
	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, s.releaseClaim(ctx, board.String(), "w1"))
	claimed, err = s.tryClaim(ctx, board.String(), "w2")
	require.NoError(t, err)
	require.True(t, claimed)

	reqBody := releaseJobRequest{WorkerID: "w1", Board: board.String()}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/release", reqBody)
	require.Equal(t, http.StatusOK, w.Code)

	claimed, err = s.tryClaim(ctx, board.String(), "w3")
	require.NoError(t, err)
	require.False(t, claimed, "w2's claim must survive w1's late release")
}

func TestHandleReleaseJob_InvalidBody(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/release", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleReleaseJob_MissingWorkerID(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	reqBody := releaseJobRequest{Board: board.String()}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/release", reqBody)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleReleaseJob_InvalidBoard(t *testing.T) {
	s := testServer(t)
	reqBody := releaseJobRequest{WorkerID: "w1", Board: "garbage"}
	w := doRequest(t, s, http.MethodPost, "/api/jobs/release", reqBody)
	require.Equal(t, http.StatusBadRequest, w.Code)
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

// A board with a row but no evaluation yet must read as "not found", not as a real score of 0.
func TestHandleGetBoard_NotLearnedYet(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	target := "/api/boards?board=" + url.QueryEscape(board.Board().String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetBoard_Success(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: 20, Score: 2}))

	target := "/api/boards?board=" + url.QueryEscape(board.Board().String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp evaluationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Depth and confidence are not stored: a level-20 search of a 12-disc board is 20@73%.
	require.Equal(t, evaluationResponse{Level: 20, Depth: 20, Confidence: 73, Score: 2, Source: evaluationSourceEdax}, resp)
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
		require.NoError(t, s.repo.SaveEvaluation(ctx, child, db.Evaluation{Level: 20, Score: 1}))
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

// A forced-pass board is never stored directly; its evaluation is the post-pass board's stored
// evaluation, negated.
func TestHandleGetBoard_ResolvesForcedPass(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testPassRequiredBoard(t)
	passed, err := board.DoMove(othello.PassMove)
	require.NoError(t, err)
	normalizedPassed := passed.Normalize()

	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{normalizedPassed}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, normalizedPassed, db.Evaluation{Level: 20, Score: 5}))

	target := "/api/boards?board=" + url.QueryEscape(board.String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp evaluationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// The search params come from the board that was actually searched, i.e. the one after the pass.
	depth, confidence := edax.SearchParams(normalizedPassed.CountDiscs(), 20)
	require.Equal(t,
		evaluationResponse{Level: 20, Depth: depth, Confidence: confidence, Score: -5, Source: evaluationSourceEdax},
		resp)
}

// A game-over board returns its actual final score instead of "not found".
func TestHandleGetBoard_GameOver(t *testing.T) {
	s := testServer(t)

	var black, white uint64
	for i := range uint(40) {
		black |= 1 << i
	}
	for i := uint(40); i < 64; i++ {
		white |= 1 << i
	}
	board, err := othello.NewBoard(black, white)
	require.NoError(t, err)
	require.False(t, board.HasMoves())
	passed, err := board.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.False(t, passed.HasMoves())

	target := "/api/boards?board=" + url.QueryEscape(board.String())
	w := doRequest(t, s, http.MethodGet, target, nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp evaluationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, evaluationResponse{Score: board.FinalScore(), Source: evaluationSourceFinal}, resp)
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

// TestHandleStats_ReturnsCounts covers the DB fallback: with no book_stats hash in Redis (first
// boot, Redis flushed), the stats are queried directly.
func TestHandleStats_ReturnsCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	w := doRequest(t, s, http.MethodGet, "/api/stats", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var entries []statEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	require.Contains(t, entries, statEntry{DiscCount: 12, Count: 1})
}

// TestHandleStats_ServesFromBookStatsHash proves the endpoint reads the rebuilt hash rather than the
// DB: a board added after the rebuild is not reported.
func TestHandleStats_ServesFromBookStatsHash(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board12 := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board12}))
	require.NoError(t, s.rebuildBookStats(ctx))

	board13 := testBoard(t, 13)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board13}))

	w := doRequest(t, s, http.MethodGet, "/api/stats", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var entries []statEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	require.Equal(t, []statEntry{{DiscCount: 12, Count: 1}}, entries)
}

// TestHandleStats_ReportsDerivedSearchParams checks that a learned board is reported by the search
// it got rather than by the level that was asked for.
func TestHandleStats_ReportsDerivedSearchParams(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	board := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: 20, Score: 2}))

	w := doRequest(t, s, http.MethodGet, "/api/stats", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var entries []statEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	require.Contains(t, entries, statEntry{DiscCount: 12, Depth: 20, Confidence: 73, Count: 1})
}

// TestStatEntries_MergesLevelsThatMeanTheSameSearch covers the merge and the ordering: at 44 discs
// every level from 10 up solves the game outright (20@100%), so those levels are one entry, sorted
// after the shallower searches and after the unlearned boards.
func TestStatEntries_MergesLevelsThatMeanTheSameSearch(t *testing.T) {
	entries := statEntries([]db.LevelStat{
		{DiscCount: 44, Level: 10, Count: 3},
		{DiscCount: 44, Level: 12, Count: 5},
		{DiscCount: 44, Level: 8, Count: 2},
		{DiscCount: 44, Level: 0, Count: 7},
	})

	require.Equal(t, []statEntry{
		{DiscCount: 44, Depth: 0, Confidence: 0, Count: 7},
		{DiscCount: 44, Depth: 8, Confidence: 100, Count: 2},
		{DiscCount: 44, Depth: 20, Confidence: 100, Count: 8},
	}, entries)
}

func TestHandleListWorkers_Empty(t *testing.T) {
	s := testServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/workers", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `[]`, w.Body.String())
}

func TestHandleListWorkers_ReturnsActiveWorkers(t *testing.T) {
	s := testServer(t)
	require.NoError(t, s.heartbeat(context.Background(), "w1", "host-1", "commit-1", ""))

	w := doRequest(t, s, http.MethodGet, "/api/workers", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var workers []workerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &workers))
	require.Len(t, workers, 1)
	require.Equal(t, "w1", workers[0].ID)
	require.Equal(t, "host-1", workers[0].Hostname)
	require.Equal(t, "commit-1", workers[0].GitCommit)
}
