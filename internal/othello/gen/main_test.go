package main

import (
	"testing"

	"github.com/lk16/flippy/internal/othello"
	"github.com/stretchr/testify/require"
)

func TestExplore_RecordsTargetDiscBoard(t *testing.T) {
	board := boardWithDiscs(t, targetDiscs)

	found := make(map[string]struct{})
	explore(board, make(map[othello.Board]bool), found)

	require.Contains(t, found, board.Normalize().String())
	require.Len(t, found, 1)
}

func TestExplore_SkipsAlreadyVisited(t *testing.T) {
	board := boardWithDiscs(t, targetDiscs)

	visited := map[othello.Board]bool{board: true}
	found := make(map[string]struct{})
	explore(board, visited, found)

	require.Empty(t, found)
}

func TestExplore_StopsWhenGameEndsEarly(t *testing.T) {
	// A board with no legal moves for either player, below targetDiscs
	// discs: the game is over, so no targetDiscs board can be reached.
	full := ^uint64(0)
	board, err := othello.NewBoard(full, 0, othello.Black)
	require.NoError(t, err)

	found := make(map[string]struct{})
	explore(board, make(map[othello.Board]bool), found)

	require.Empty(t, found)
}

func TestExplore_FromStart_MatchesPrecomputedCount(t *testing.T) {
	found := make(map[string]struct{})
	explore(othello.NewBoardStart(), make(map[othello.Board]bool), found)

	require.Len(t, found, 67239)
}

func TestExplore_SkipsTargetDiscBoardWithNoLegalMove(t *testing.T) {
	// A real 12-disc board reached by legal play from start where black
	// (to move) has no legal move, only white does after a pass.
	board, err := othello.ParseBoard("0000001c183000800000000000c04020-b")
	require.NoError(t, err)
	require.Equal(t, targetDiscs, board.CountDiscs())
	require.False(t, board.HasMoves(), "test board must have no legal move for this test to be meaningful")

	found := make(map[string]struct{})
	explore(board, make(map[othello.Board]bool), found)

	require.Empty(t, found)
}

// boardWithDiscs returns any legal board with exactly n discs, reached by
// repeatedly playing the first available legal move (or pass) from start.
func boardWithDiscs(t *testing.T, n int) othello.Board {
	t.Helper()

	board := othello.NewBoardStart()
	for board.CountDiscs() < n {
		if !board.HasMoves() {
			next, err := board.DoMove(othello.PassMove)
			require.NoError(t, err)
			board = next
			continue
		}

		children := board.Children()
		require.NotEmpty(t, children)
		board = children[0]
	}

	return board
}
