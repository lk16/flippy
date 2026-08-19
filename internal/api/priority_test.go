package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

// drainPriority pops every entry currently in the priority queue.
func drainPriority(t *testing.T, s *Server) []priorityEntry {
	t.Helper()

	var entries []priorityEntry
	for {
		entry, ok, err := s.dequeuePriority(context.Background())
		require.NoError(t, err)
		if !ok {
			return entries
		}
		entries = append(entries, entry)
	}
}

// TestClaimJob_PriorityDrainedFirst verifies that priority-queue boards are returned before
// any ListLearnable candidates.
func TestClaimJob_PriorityDrainedFirst(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Put a learnable board in the DB so ListLearnable has something to offer.
	dbBoard := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{dbBoard}))

	// Enqueue a different board as priority.
	pBoard := testBoard(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel, ""))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, pBoard, job.Board)
	require.Equal(t, PriorityLevel, job.Level)

	// The next claim falls back to the DB board.
	job, ok, err = s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, dbBoard, job.Board)
}

// TestClaimJob_PrioritySkipsNoMovesBoard ensures boards with no legal move are skipped in the priority path.
func TestClaimJob_PrioritySkipsNoMovesBoard(t *testing.T) {
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
	require.NoError(t, s.enqueuePriority(ctx, normalizedNoMove.String(), PriorityLevel, ""))

	// Add a normal DB board too.
	dbBoard := testBoard(t, 12)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{dbBoard}))

	// The no-move board is skipped; the claim falls through to the DB board.
	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, dbBoard, job.Board)
}

// TestClaimJob_PriorityDeduplicates verifies that enqueuePriority skips duplicates.
func TestClaimJob_PriorityDeduplicates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	pBoard := testBoard(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel, ""))
	require.NoError(t, s.enqueuePriority(ctx, pBoard.String(), PriorityLevel, "")) // duplicate — should be ignored

	require.Len(t, drainPriority(t, s), 1)
}

// TestDequeuePriority_DropsEntriesFromDeadConnections verifies that an entry queued by a since-
// closed websocket connection is discarded at dequeue and removed from the pending set.
func TestDequeuePriority_DropsEntriesFromDeadConnections(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	connID := s.registerConn()
	board := testBoard(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, board.String(), PriorityLevel, connID))
	s.unregisterConn(connID)

	_, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	// Removed from the pending set too, so the board can be re-queued.
	isMember, err := s.redis.SIsMember(ctx, priorityPendingKey, board.String()).Result()
	require.NoError(t, err)
	require.False(t, isMember)
}

// TestDequeuePriority_KeepsEntriesFromLiveConnections verifies that entries whose connection is
// still open come through, including when they sit behind a dead connection's entry.
func TestDequeuePriority_KeepsEntriesFromLiveConnections(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	deadConn := s.registerConn()
	liveConn := s.registerConn()

	deadBoard := testBoard(t, 14)
	liveBoard := testBoard(t, 15)
	require.NoError(t, s.enqueuePriority(ctx, deadBoard.String(), PriorityLevel, deadConn))
	require.NoError(t, s.enqueuePriority(ctx, liveBoard.String(), PriorityLevel, liveConn))
	s.unregisterConn(deadConn)

	entry, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, liveBoard.String(), entry.Board)

	_, ok, err = s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestDequeuePriority_DedupeIsByBoardOnly documents the chosen semantics: dedupe keys on the board
// alone, so a board queued first by a connection that then dies is dropped even though a second,
// still-live connection asked for it too. That client's next analyze_request re-queues the board,
// so the loss heals itself.
func TestDequeuePriority_DedupeIsByBoardOnly(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	connA := s.registerConn()
	connB := s.registerConn()
	board := testBoard(t, 14)

	require.NoError(t, s.enqueuePriority(ctx, board.String(), PriorityLevel, connA))
	require.NoError(t, s.enqueuePriority(ctx, board.String(), PriorityLevel, connB)) // deduped: still tagged connA
	s.unregisterConn(connA)

	_, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.False(t, ok)

	// connB re-requesting after the drop queues the board again.
	require.NoError(t, s.enqueuePriority(ctx, board.String(), PriorityLevel, connB))
	entry, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board.String(), entry.Board)
}

// TestDequeuePriority_UntaggedEntriesAreNeverDropped covers entries with no connection ID (legacy
// format): they cannot be matched to a live connection, so they always come through.
func TestDequeuePriority_UntaggedEntriesAreNeverDropped(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	require.NoError(t, s.enqueuePriority(ctx, board.String(), PriorityLevel, ""))

	entry, ok, err := s.dequeuePriority(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, board.String(), entry.Board)
}

// TestHandleSubmitJobResult_PriorityHighDiscSkipsPersistence verifies that a priority job for
// a board with > MaxSavableDiscs does not attempt a DB write but does cache the result ephemerally.
func TestHandleSubmitJobResult_PriorityHighDiscSkipsPersistence(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Use a board with many discs to exceed MaxSavableDiscs (30).
	board := testBoard(t, 35)
	require.Greater(t, board.CountDiscs(), book.MaxSavableDiscs)

	require.Equal(t, 200, submitPriorityResult(t, s, board, TargetLevel(board.CountDiscs()), 2).Code)

	// Board should not have been inserted into boards table.
	_, err := s.repo.GetBoard(ctx, board.Board())
	require.Error(t, err)

	// But the ephemeral cache should have it.
	cached, ok, err := s.getAnalysisResult(ctx, board.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, cached.Score)
}

// submitPriorityResult claims board as a priority job and submits an evaluation at level for it,
// returning the response recorder.
func submitPriorityResult(t *testing.T, s *Server, board othello.NormalizedBoard, level, score int) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, s.setPriorityClaim(ctx, board.String()))
	claimed, err := s.tryClaim(ctx, board.String(), "w1")
	require.NoError(t, err)
	require.True(t, claimed)

	depth, confidence := edax.SearchParams(board.CountDiscs(), level)
	return doRequest(t, s, "POST", "/api/jobs/result", jobResultRequest{
		WorkerID: "w1", Board: board.String(), Level: level,
		Depth: depth, Confidence: confidence, Score: score,
	})
}

// TestHandleSubmitJobResult_PriorityLowDiscPersists verifies that a priority job for a board
// with <= MaxSavableDiscs (which already has a row), searched at its target level, saves the
// evaluation to the DB.
func TestHandleSubmitJobResult_PriorityLowDiscPersists(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))

	target := TargetLevel(board.CountDiscs())
	require.Equal(t, 200, submitPriorityResult(t, s, board, target, 6).Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: target, Score: 6}, eval)
}

// TestHandleSubmitJobResult_PriorityLowDiscNoRowAddsAndSaves verifies the AddBoards+retry path:
// when a priority <=30-disc board searched at its target level has no existing row, one is created.
func TestHandleSubmitJobResult_PriorityLowDiscNoRowAddsAndSaves(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	// Do NOT call AddBoards; the board has no row.

	target := TargetLevel(board.CountDiscs())
	require.Equal(t, 200, submitPriorityResult(t, s, board, target, -4).Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: target, Score: -4}, eval)
}

// TestHandleSubmitJobResult_PriorityBelowTargetNotPersisted verifies the level floor: a priority
// result shallower than the board's target level leaves the existing row unevaluated, so the
// intermediate rungs of the frontend's level ladder never land in the book.
func TestHandleSubmitJobResult_PriorityBelowTargetNotPersisted(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.Less(t, PriorityLevel, TargetLevel(board.CountDiscs()))

	require.Equal(t, 200, submitPriorityResult(t, s, board, PriorityLevel, 6).Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval, "row stays unevaluated")

	// The result is still served back: it lives in the ephemeral analysis cache.
	cached, ok, err := s.getAnalysisResult(ctx, board.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 6, cached.Score)
}

// TestHandleSubmitJobResult_PriorityBelowTargetSchedulesBoardForLearning verifies that a
// below-target priority result on an unknown savable board creates a row with an empty evaluation,
// so ListLearnable picks the board up later — without seeding the book with the shallow score.
func TestHandleSubmitJobResult_PriorityBelowTargetSchedulesBoardForLearning(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	// Do NOT call AddBoards; the board has no row.

	require.Equal(t, 200, submitPriorityResult(t, s, board, PriorityLevel, 6).Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{}, eval, "row exists but stays unevaluated")
}

// TestHandleSubmitJobResult_PriorityBelowTargetDoesNotDowngradeExistingRow verifies the
// insert-if-absent guarantee: a board with a real evaluation is untouched by a shallow result.
func TestHandleSubmitJobResult_PriorityBelowTargetDoesNotDowngradeExistingRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 14)
	target := TargetLevel(board.CountDiscs())
	require.NoError(t, s.repo.AddBoards(ctx, []othello.NormalizedBoard{board}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, board, db.Evaluation{Level: target, Score: 5}))

	require.Equal(t, 200, submitPriorityResult(t, s, board, PriorityLevel, 6).Code)

	eval, err := s.repo.GetBoard(ctx, board.Board())
	require.NoError(t, err)
	require.Equal(t, db.Evaluation{Level: target, Score: 5}, eval)
}

// TestHandleSubmitJobResult_PriorityBelowTargetHighDiscAddsNoRow verifies that boards above
// MaxSavableDiscs are not scheduled for learning either.
func TestHandleSubmitJobResult_PriorityBelowTargetHighDiscAddsNoRow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	board := testBoard(t, 35)
	require.Greater(t, board.CountDiscs(), book.MaxSavableDiscs)

	require.Equal(t, 200, submitPriorityResult(t, s, board, PriorityLevel, 6).Code)

	_, err := s.repo.GetBoard(ctx, board.Board())
	require.ErrorIs(t, err, db.ErrBoardNotFound)
}

// TestHandleSubmitJobResult_NonPriorityBoardNotFoundStill404 ensures the existing 404 path
// for non-priority jobs is untouched by the priority changes.
func TestHandleSubmitJobResult_NonPriorityBoardNotFoundStill404(t *testing.T) {
	s := testServer(t)
	board := testBoard(t, 12)
	// No priority claim, no DB row.
	reqBody := jobResultRequest{WorkerID: "w1", Board: board.String(), Level: TargetLevel(12), Score: 0}
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
