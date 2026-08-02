package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// rotateBoard applies spatial symmetry r to a board's discs directly,
// independent of whose turn it is. This is used to check that Normalize
// collapses all 8 symmetries of a board to the same NormalizedBoard.
func rotateBoard(b Board, r int) Board {
	return Board{
		black: rotateBits(b.black, r),
		white: rotateBits(b.white, r),
		turn:  b.turn,
	}
}

func testBoards(t *testing.T) []Board {
	t.Helper()

	boards := []Board{NewBoardStart()}

	board := NewBoardStart()
	for _, move := range []int{19, 18, 17, 9, 1, 0} {
		var err error
		board, err = board.DoMove(move)
		require.NoError(t, err)
		boards = append(boards, board)
	}

	return boards
}

func TestBoard_Normalize_Symmetry(t *testing.T) {
	for _, board := range testBoards(t) {
		want := board.Normalize()

		for r := range 8 {
			rotated := rotateBoard(board, r)
			require.Equal(t, want, rotated.Normalize(), "rotation %d should normalize the same way", r)
		}
	}
}

func TestBoard_Normalize_Idempotent(t *testing.T) {
	for _, board := range testBoards(t) {
		nb := board.Normalize()
		require.True(t, nb.Board().IsNormalized())
		require.Equal(t, nb, nb.Board().Normalize())
	}
}

func TestBoard_Normalize_PreservesTurn(t *testing.T) {
	for _, board := range testBoards(t) {
		require.Equal(t, board.Turn(), board.Normalize().Turn())
	}
}

func TestBoard_Normalize_PreservesDiscCount(t *testing.T) {
	for _, board := range testBoards(t) {
		require.Equal(t, board.CountDiscs(), board.Normalize().CountDiscs())
	}
}

func TestNewNormalizedBoard(t *testing.T) {
	board := testBoards(t)[len(testBoards(t))-1]
	rotated := rotateBoard(board, 1)
	require.NotEqual(t, board, rotated, "test board must not be symmetric for this test to be meaningful")

	_, err := NewNormalizedBoard(rotated)
	require.Error(t, err, "an arbitrary rotation should not already be in canonical form")

	nb, err := NewNormalizedBoard(rotated.Normalize().Board())
	require.NoError(t, err)
	require.Equal(t, rotated.Normalize(), nb)
}

func TestNormalizedBoard_Accessors(t *testing.T) {
	nb := NewBoardStart().Normalize()

	require.Equal(t, nb.Board().Black(), nb.Black())
	require.Equal(t, nb.Board().White(), nb.White())
	require.Equal(t, nb.Board().Turn(), nb.Turn())
	require.Equal(t, nb.Board().CountDiscs(), nb.CountDiscs())
	require.Equal(t, nb.Board().HasMoves(), nb.HasMoves())
	require.Equal(t, nb.Board().String(), nb.String())
}
