package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// TestClaimJobs_PriorityDrainedFirst verifies that priority-queue boards are returned before
// any ListLearnable candidates.
func TestClaimJobs_PriorityDrainedFirst(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Put a learnabled board in the DB so ListLearnable has something to offer.
	dbBoard := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{dbBoard}))

	// Enqueue a different board as priority.
	pBoard := testBoard(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel))

	jobs, err := s.claimJobs(ctx, "worker-1", 2)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(jobs), 1)
	// First job must be the priority board.
	require.Equal(t, pBoard, jobs[0].Board)
	require.Equal(t, PriorityLevel, jobs[0].Level)
}

// TestClaimJobs_PriorityRespectsTotalCount ensures priority + DB jobs together don't exceed count.
func TestClaimJobs_PriorityRespectsTotalCount(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Add enough DB boards.
	dbBoards := testDistinctBoards(t, 12, 3)
	require.NoError(t, s.repo.AddBoards(ctx, dbBoards))

	// Enqueue 2 priority boards.
	pBoards := testDistinctBoards(t, 14, 2)
	for _, b := range pBoards {
		require.NoError(t, s.enqueuePriority(ctx, b.String(), PriorityLevel))
	}

	jobs, err := s.claimJobs(ctx, "worker-1", 2)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
}

// TestClaimJobs_PrioritySkipsNoMovesBoard ensures boards with no legal move are skipped in the priority path.
func TestClaimJobs_PrioritySkipsNoMovesBoard(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Construct a no-legal-move board.
	var blackBits, whiteBits uint64
	for i := range uint(40) {
		blackBits |= 1 << i
	}
	for i := uint(40); i < 64; i++ {
		whiteBits |= 1 << i
	}
	noMoveBoard, err := othello.NewBoard(blackBits, whiteBits, othello.Black)
	require.NoError(t, err)
	require.False(t, noMoveBoard.HasMoves())
	normalizedNoMove := noMoveBoard.Normalize()

	// Enqueue it in the priority queue (server shouldn't claim it).
	require.NoError(t, s.enqueuePriority(ctx, normalizedNoMove.String(), PriorityLevel))

	// Add a normal DB board too.
	dbBoard := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{dbBoard}))

	jobs, err := s.claimJobs(ctx, "worker-1", 2)
	require.NoError(t, err)
	// The no-move board must not be in the jobs.
	for _, j := range jobs {
		require.NotEqual(t, normalizedNoMove, j.Board)
	}
}

// TestClaimJobs_PriorityDeduplicates verifies that enqueuePriority skips duplicates.
func TestClaimJobs_PriorityDeduplicates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	pBoard := testBoard(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel))
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel)) // duplicate — should be ignored

	entries, err := s.dequeuePriority(ctx, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// TestHandleSubmitJobResult_PriorityHighDiscSkipsPersistence verifies that a priority job for
// a board with > MaxSavableDiscs does not attempt a DB write but does cache the result ephemerally.
func TestHandleSubmitJobResult_PriorityHighDiscSkipsPersistence(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Use a board with many discs to exceed MaxSavableDiscs (30).
	board := testBoard(t, 35)
	require.Greater(t, board.CountDiscs(), book.MaxSavableDiscs)

	// Simulate a priority claim.
	require.NoError(t, s.setPriorityClaim(ctx, board.String()))

	// Also set a regular claim (as claimJobs would do via tryClaim).
	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	reqBody := jobResultRequest{
		WorkerID: "w1", Board: board.String(), Level: PriorityLevel,
		Depth: PriorityLevel, Confidence: 100, Score: 2,
	}
	w := doRequest(t, s, "POST", "/api/jobs/result", reqBody)
	require.Equal(t, 200, w.Code)

	// Board should not have been inserted into boards table.
	_, err = s.repo.GetBoard(ctx, board.Board())
	require.Error(t, err)

	// But the ephemeral cache should have it.
	cached, ok, err := s.getAnalysisResult(ctx, board.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, cached.Score)
}

// TestHandleSubmitJobResult_PriorityLowDiscPersists verifies that a priority job for a board
// with <= MaxSavableDiscs (which already has a row) saves the evaluation to the DB.
func TestHandleSubmitJobResult_PriorityLowDiscPersists(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	require.NoError(t, s.setPriorityClaim(ctx, board.String()))
	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	reqBody := jobResultRequest{
		WorkerID: "w1", Board: board.String(), Level: PriorityLevel,
		Depth: PriorityLevel, Confidence: 100, Score: 6,
	}
	w := doRequest(t, s, "POST", "/api/jobs/result", reqBody)
	require.Equal(t, 200, w.Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: PriorityLevel, Score: 6}, eval)
}

// TestHandleSubmitJobResult_PriorityLowDiscNoRowAddsAndSaves verifies the AddBoards+retry path:
// when a priority <=30-disc board has no existing row, one is created.
func TestHandleSubmitJobResult_PriorityLowDiscNoRowAddsAndSaves(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	// Do NOT call AddBoards; the board has no row.

	require.NoError(t, s.setPriorityClaim(ctx, board.String()))
	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	reqBody := jobResultRequest{
		WorkerID: "w1", Board: board.String(), Level: PriorityLevel,
		Depth: PriorityLevel, Confidence: 100, Score: -4,
	}
	w := doRequest(t, s, "POST", "/api/jobs/result", reqBody)
	require.Equal(t, 200, w.Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: PriorityLevel, Score: -4}, eval)
}

// TestHandleSubmitJobResult_NonPriorityBoardNotFoundStill404 ensures the existing 404 path
// for non-priority jobs is untouched by the priority changes.
func TestHandleSubmitJobResult_NonPriorityBoardNotFoundStill404(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	// No priority claim, no DB row.
	reqBody := jobResultRequest{WorkerID: "w1", Board: board.String(), Level: 24, Depth: 24, Confidence: 73, Score: 0}
	w := doRequest(t, s, "POST", "/api/jobs/result", reqBody)
	require.Equal(t, 404, w.Code)
}

// TestLookupEvaluation_EphemeralCacheFallback verifies that lookupEvaluation returns an
// analysis result stored in the ephemeral cache when the DB and minimax cache both miss.
func TestLookupEvaluation_EphemeralCacheFallback(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 20)
	eval := evaluationResponse{Level: PriorityLevel, Depth: PriorityLevel, Confidence: 100, Score: 8, Source: evaluationSourceEdax}
	s.setAnalysisResult(ctx, board.String(), eval)

	result, ok, err := s.lookupEvaluation(ctx, board.Board())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, eval, result)
}
