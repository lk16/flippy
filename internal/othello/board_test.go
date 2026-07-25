package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBoardStart(t *testing.T) {
	board := NewBoardStart()

	require.Equal(t, Black, board.Turn())
	require.Equal(t, 4, board.CountDiscs())
	require.True(t, board.HasMoves())
}

func TestNewBoard_Overlap(t *testing.T) {
	_, err := NewBoard(0x1, 0x1, Black)
	require.Error(t, err)
}

func TestNewBoardEmpty(t *testing.T) {
	board := NewBoardEmpty()

	require.Equal(t, Black, board.Turn())
	require.Equal(t, 0, board.CountDiscs())
}

func TestColor_String(t *testing.T) {
	require.Equal(t, "black", Black.String())
	require.Equal(t, "white", White.String())
}

func TestBoard_Moves_Start(t *testing.T) {
	board := NewBoardStart()

	// Standard Othello opening moves for black: d3, c4, f5, e6.
	wantMoves := []int{19, 26, 37, 44}
	for _, move := range wantMoves {
		require.True(t, board.IsValidMove(move), "move %d should be legal", move)
	}

	invalidMoves := []int{0, 7, 56, 63, 27, 28, 35, 36}
	for _, move := range invalidMoves {
		require.False(t, board.IsValidMove(move), "move %d should be illegal", move)
	}

	require.False(t, board.IsValidMove(PassMove), "pass should be illegal when moves exist")
}

func TestBoard_IsValidMove_OutOfRange(t *testing.T) {
	board := NewBoardStart()

	require.False(t, board.IsValidMove(-5))
	require.False(t, board.IsValidMove(64))
}

func TestBoard_PassDetection(t *testing.T) {
	// A board with no empty squares for black has no legal moves, so a pass
	// is the only legal move.
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0, Black)
	require.NoError(t, err)

	require.False(t, board.HasMoves())
	require.True(t, board.IsValidMove(PassMove))
	require.False(t, board.IsValidMove(0))
}

func TestBoard_DoMove(t *testing.T) {
	board := NewBoardStart()

	next, err := board.DoMove(19)
	require.NoError(t, err)
	require.Equal(t, White, next.Turn())
	require.NotEqual(t, board.Black(), next.Black())
	require.Greater(t, next.CountDiscs(), board.CountDiscs())
}

func TestBoard_DoMove_Invalid(t *testing.T) {
	board := NewBoardStart()

	_, err := board.DoMove(0)
	require.Error(t, err)
}

func TestBoard_DoMove_Pass(t *testing.T) {
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0, Black)
	require.NoError(t, err)

	passed, err := board.DoMove(PassMove)
	require.NoError(t, err)

	// A pass flips whose turn it is, but never changes who owns which
	// square: black/white discs are absolute colors, not mover-relative.
	require.Equal(t, board.Black(), passed.Black())
	require.Equal(t, board.White(), passed.White())
	require.Equal(t, White, passed.Turn())
}

func TestBoard_DoMove_PassInvalidWhenMovesExist(t *testing.T) {
	board := NewBoardStart()

	_, err := board.DoMove(PassMove)
	require.Error(t, err)
}

func TestBoard_Children(t *testing.T) {
	board := NewBoardStart()

	children := board.Children()
	require.Len(t, children, 4)

	seen := make(map[Board]bool)
	for _, child := range children {
		require.Equal(t, White, child.Turn())
		seen[child] = true
	}
	require.Len(t, seen, 4)
}

func TestBoard_Children_NoMoves(t *testing.T) {
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0, Black)
	require.NoError(t, err)

	require.Empty(t, board.Children())
}

func TestBoard_FinalScore(t *testing.T) {
	// 40 black discs, 24 white discs, black to move: black is ahead.
	black := uint64(0xFFFFFFFFFF000000)
	white := uint64(0x0000000000FFFFFF)
	board, err := NewBoard(black, white, Black)
	require.NoError(t, err)

	require.Equal(t, 64-2*24, board.FinalScore())

	board, err = NewBoard(black, white, White)
	require.NoError(t, err)
	require.Equal(t, -64+2*24, board.FinalScore())
}

func TestBoard_FinalScore_Tie(t *testing.T) {
	black := uint64(0x00000000FFFF0000)
	white := uint64(0x0000FFFF00000000)
	board, err := NewBoard(black, white, Black)
	require.NoError(t, err)

	require.Equal(t, 0, board.FinalScore())
}

func TestBoard_String(t *testing.T) {
	board := NewBoardStart()

	require.Len(t, board.String(), 16+16+2)
	require.Equal(t, "-b", board.String()[32:])

	next, err := board.DoMove(19)
	require.NoError(t, err)
	require.Equal(t, "-w", next.String()[32:])
}
