package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBoardStart(t *testing.T) {
	board := NewBoardStart()

	require.Equal(t, uint64(startPlayerDiscs), board.Player())
	require.Equal(t, uint64(startOpponentDiscs), board.Opponent())
	require.Equal(t, 4, board.CountDiscs())
	require.True(t, board.HasMoves())
}

func TestNewBoard_Overlap(t *testing.T) {
	_, err := NewBoard(0x1, 0x1)
	require.Error(t, err)
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
	// A board where the mover owns every square has no legal moves, so a pass
	// is the only legal move.
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	require.False(t, board.HasMoves())
	require.True(t, board.IsValidMove(PassMove))
	require.False(t, board.IsValidMove(0))
}

func TestBoard_DoMove(t *testing.T) {
	board := NewBoardStart()

	next, err := board.DoMove(19)
	require.NoError(t, err)

	// The board is always seen from the player to move, so the discs that were
	// the mover's are the opponent's afterwards.
	require.NotEqual(t, board.Player(), next.Player())
	require.Greater(t, next.CountDiscs(), board.CountDiscs())
	require.Equal(t, board.Player()|(uint64(1)<<19)|flippedDiscs(board.Player(), board.Opponent(), 19), next.Opponent())
}

func TestBoard_DoMove_Invalid(t *testing.T) {
	board := NewBoardStart()

	_, err := board.DoMove(0)
	require.Error(t, err)
}

func TestBoard_DoMove_Pass(t *testing.T) {
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	passed, err := board.DoMove(PassMove)
	require.NoError(t, err)

	// A pass changes no square's owner, only whose turn it is -- which is the
	// same thing as swapping the two bitboards.
	require.Equal(t, board.Player(), passed.Opponent())
	require.Equal(t, board.Opponent(), passed.Player())
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
		seen[child] = true
	}
	require.Len(t, seen, 4)
}

func TestBoard_Children_NoMoves(t *testing.T) {
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	require.Empty(t, board.Children())
}

func TestBoard_FinalScore(t *testing.T) {
	// 40 discs for the mover, 24 for the opponent: the mover is ahead.
	mover := uint64(0xFFFFFFFFFF000000)
	other := uint64(0x0000000000FFFFFF)
	board, err := NewBoard(mover, other)
	require.NoError(t, err)

	require.Equal(t, 64-2*24, board.FinalScore())

	board, err = NewBoard(other, mover)
	require.NoError(t, err)
	require.Equal(t, -64+2*24, board.FinalScore())
}

func TestBoard_FinalScore_Tie(t *testing.T) {
	board, err := NewBoard(0x00000000FFFF0000, 0x0000FFFF00000000)
	require.NoError(t, err)

	require.Equal(t, 0, board.FinalScore())
}

func TestBoard_String(t *testing.T) {
	board := NewBoardStart()

	require.Len(t, board.String(), BoardStringLength)
	require.Equal(t, "00000008100000000000001008000000", board.String())
}

func TestParseBoard_RoundTrip(t *testing.T) {
	board, err := NewBoardStart().DoMove(19)
	require.NoError(t, err)

	parsed, err := ParseBoard(board.String())
	require.NoError(t, err)
	require.Equal(t, board, parsed)
}

func TestParseBoard_InvalidLength(t *testing.T) {
	_, err := ParseBoard("too-short")
	require.Error(t, err)

	// The old 34-character format, black/white discs plus a turn suffix.
	_, err = ParseBoard("00000008100000000000001008000000-b")
	require.Error(t, err)
}

func TestParseBoard_InvalidPlayerHex(t *testing.T) {
	_, err := ParseBoard("zzzzzzzzzzzzzzzz0000000000000000")
	require.Error(t, err)
}

func TestParseBoard_InvalidOpponentHex(t *testing.T) {
	_, err := ParseBoard("0000000000000000zzzzzzzzzzzzzzzz")
	require.Error(t, err)
}

func TestParseBoard_Overlap(t *testing.T) {
	_, err := ParseBoard("ffffffffffffffffffffffffffffffff")
	require.Error(t, err)
}

func TestBoard_Bytes_RoundTrip(t *testing.T) {
	board, err := NewBoardStart().DoMove(19)
	require.NoError(t, err)

	buf := board.Bytes()
	require.Len(t, buf, BoardBytesLength)

	parsed, err := ParseBoardBytes(buf)
	require.NoError(t, err)
	require.Equal(t, board, parsed)
}

func TestBoard_Bytes_PlayerFirst(t *testing.T) {
	board, err := NewBoard(0x00000000000000FF, 0xFF00000000000000)
	require.NoError(t, err)

	require.Equal(t,
		[]byte{0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0},
		board.Bytes())
}

func TestParseBoardBytes_InvalidLength(t *testing.T) {
	_, err := ParseBoardBytes([]byte{1, 2, 3})
	require.Error(t, err)

	// The old 17-byte format, black/white discs plus a turn byte.
	_, err = ParseBoardBytes(make([]byte, 17))
	require.Error(t, err)
}

func TestParseBoardBytes_Overlap(t *testing.T) {
	buf := make([]byte, BoardBytesLength)
	for i := range 16 {
		buf[i] = 0xff
	}
	_, err := ParseBoardBytes(buf)
	require.Error(t, err)
}
