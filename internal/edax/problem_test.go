package edax

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

func TestProblemLine_Start(t *testing.T) {
	// Black to move at the start, so the trailing marker is "X".
	got := problemLine(othello.NewBoardStart())

	want := "---------------------------OX------XO--------------------------- X;\n"
	require.Equal(t, want, got)
}

func TestProblemLine_PreservesRealTurnAndColors(t *testing.T) {
	board, err := othello.NewBoardStart().DoMove(19) // d3, white to move next
	require.NoError(t, err)
	require.Equal(t, othello.White, board.Turn())

	got := problemLine(board)

	// Squares must reflect the board's literal colors (black='X',
	// white='O'), not be relabeled around whoever is to move.
	want := "-------------------X-------XX------XO--------------------------- O;\n"
	require.Equal(t, want, got)
}

func TestProblemLine_TurnMarkerMatchesBoardTurn(t *testing.T) {
	black := othello.NewBoardStart()
	require.Equal(t, othello.Black, black.Turn())
	require.Equal(t, " X;\n", problemLine(black)[64:])

	white, err := black.DoMove(19)
	require.NoError(t, err)
	require.Equal(t, othello.White, white.Turn())
	require.Equal(t, " O;\n", problemLine(white)[64:])
}
