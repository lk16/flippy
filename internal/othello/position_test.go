package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStartPosition(t *testing.T) {
	position := NewStartPosition()

	require.Equal(t, uint64(startPlayerDiscs), position.Player())
	require.Equal(t, uint64(startOpponentDiscs), position.Opponent())
	require.Equal(t, 4, position.CountDiscs())
	require.True(t, position.HasMoves())
}

func TestNewPosition_Overlap(t *testing.T) {
	_, err := NewPosition(0x1, 0x1)
	require.Error(t, err)
}

func TestColor_String(t *testing.T) {
	require.Equal(t, "black", Black.String())
	require.Equal(t, "white", White.String())
}

func TestPosition_Moves_Start(t *testing.T) {
	position := NewStartPosition()

	// Standard Othello opening moves for black: d3, c4, f5, e6.
	wantMoves := []int{19, 26, 37, 44}
	for _, move := range wantMoves {
		require.True(t, position.IsValidMove(move), "move %d should be legal", move)
	}

	invalidMoves := []int{0, 7, 56, 63, 27, 28, 35, 36}
	for _, move := range invalidMoves {
		require.False(t, position.IsValidMove(move), "move %d should be illegal", move)
	}

	require.False(t, position.IsValidMove(PassMove), "pass should be illegal when moves exist")
}

func TestPosition_IsValidMove_OutOfRange(t *testing.T) {
	position := NewStartPosition()

	require.False(t, position.IsValidMove(-5))
	require.False(t, position.IsValidMove(64))
}

func TestPosition_PassDetection(t *testing.T) {
	// A position where the mover owns every square has no legal moves, so a pass
	// is the only legal move.
	position, err := NewPosition(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	require.False(t, position.HasMoves())
	require.True(t, position.IsValidMove(PassMove))
	require.False(t, position.IsValidMove(0))
}

func TestPosition_DoMove(t *testing.T) {
	position := NewStartPosition()

	next, err := position.DoMove(19)
	require.NoError(t, err)

	// The position is always seen from the player to move, so the discs that were
	// the mover's are the opponent's afterwards.
	require.NotEqual(t, position.Player(), next.Player())
	require.Greater(t, next.CountDiscs(), position.CountDiscs())
	require.Equal(t, position.Player()|(uint64(1)<<19)|flippedDiscs(position.Player(), position.Opponent(), 19), next.Opponent())
}

func TestPosition_DoMove_Invalid(t *testing.T) {
	position := NewStartPosition()

	_, err := position.DoMove(0)
	require.Error(t, err)
}

func TestPosition_DoMove_Pass(t *testing.T) {
	position, err := NewPosition(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	passed, err := position.DoMove(PassMove)
	require.NoError(t, err)

	// A pass changes no square's owner, only whose turn it is -- which is the
	// same thing as swapping the two bitboards.
	require.Equal(t, position.Player(), passed.Opponent())
	require.Equal(t, position.Opponent(), passed.Player())
}

func TestPosition_DoMove_PassInvalidWhenMovesExist(t *testing.T) {
	position := NewStartPosition()

	_, err := position.DoMove(PassMove)
	require.Error(t, err)
}

func TestPosition_Children(t *testing.T) {
	position := NewStartPosition()

	children := position.Children()
	require.Len(t, children, 4)

	seen := make(map[Position]bool)
	for _, child := range children {
		seen[child] = true
	}
	require.Len(t, seen, 4)
}

func TestPosition_Children_NoMoves(t *testing.T) {
	position, err := NewPosition(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	require.Empty(t, position.Children())
}

func TestPosition_FinalScore(t *testing.T) {
	// 40 discs for the mover, 24 for the opponent: the mover is ahead.
	mover := uint64(0xFFFFFFFFFF000000)
	other := uint64(0x0000000000FFFFFF)
	position, err := NewPosition(mover, other)
	require.NoError(t, err)

	require.Equal(t, 64-2*24, position.FinalScore())

	position, err = NewPosition(other, mover)
	require.NoError(t, err)
	require.Equal(t, -64+2*24, position.FinalScore())
}

func TestPosition_FinalScore_Tie(t *testing.T) {
	position, err := NewPosition(0x00000000FFFF0000, 0x0000FFFF00000000)
	require.NoError(t, err)

	require.Equal(t, 0, position.FinalScore())
}

func TestPosition_String(t *testing.T) {
	position := NewStartPosition()

	require.Len(t, position.String(), PositionStringLength)
	require.Equal(t, "00000008100000000000001008000000", position.String())
}

func TestParsePosition_RoundTrip(t *testing.T) {
	position, err := NewStartPosition().DoMove(19)
	require.NoError(t, err)

	parsed, err := ParsePosition(position.String())
	require.NoError(t, err)
	require.Equal(t, position, parsed)
}

func TestParsePosition_InvalidLength(t *testing.T) {
	_, err := ParsePosition("too-short")
	require.Error(t, err)

	// The old 34-character format, black/white discs plus a turn suffix.
	_, err = ParsePosition("00000008100000000000001008000000-b")
	require.Error(t, err)
}

func TestParsePosition_InvalidPlayerHex(t *testing.T) {
	_, err := ParsePosition("zzzzzzzzzzzzzzzz0000000000000000")
	require.Error(t, err)
}

func TestParsePosition_InvalidOpponentHex(t *testing.T) {
	_, err := ParsePosition("0000000000000000zzzzzzzzzzzzzzzz")
	require.Error(t, err)
}

func TestParsePosition_Overlap(t *testing.T) {
	_, err := ParsePosition("ffffffffffffffffffffffffffffffff")
	require.Error(t, err)
}

func TestPosition_Bytes_RoundTrip(t *testing.T) {
	position, err := NewStartPosition().DoMove(19)
	require.NoError(t, err)

	buf := position.Bytes()
	require.Len(t, buf, PositionBytesLength)

	parsed, err := ParsePositionBytes(buf)
	require.NoError(t, err)
	require.Equal(t, position, parsed)
}

func TestPosition_Bytes_PlayerFirst(t *testing.T) {
	position, err := NewPosition(0x00000000000000FF, 0xFF00000000000000)
	require.NoError(t, err)

	require.Equal(t,
		[]byte{0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0},
		position.Bytes())
}

func TestParsePositionBytes_InvalidLength(t *testing.T) {
	_, err := ParsePositionBytes([]byte{1, 2, 3})
	require.Error(t, err)

	// The old 17-byte format, black/white discs plus a turn byte.
	_, err = ParsePositionBytes(make([]byte, 17))
	require.Error(t, err)
}

func TestParsePositionBytes_Overlap(t *testing.T) {
	buf := make([]byte, PositionBytesLength)
	for i := range 16 {
		buf[i] = 0xff
	}
	_, err := ParsePositionBytes(buf)
	require.Error(t, err)
}
