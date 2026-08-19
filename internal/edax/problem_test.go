package edax

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

func TestProblemLine_Start(t *testing.T) {
	got := problemLine(othello.NewStartPosition())

	want := "---------------------------OX------XO--------------------------- X;\n"
	require.Equal(t, want, got)
}

func TestProblemLine_IsMoverRelative(t *testing.T) {
	position, err := othello.NewStartPosition().DoMove(19) // d3
	require.NoError(t, err)

	got := problemLine(position)

	// 'X' is the player to move (white here, having just been played into), 'O' the opponent:
	// squares are labelled by side to move, not by disc color.
	want := "-------------------O-------OO------OX--------------------------- X;\n"
	require.Equal(t, want, got)
}

func TestProblemLine_ColorToMoveIsAlwaysX(t *testing.T) {
	position := othello.NewStartPosition()

	for range 4 {
		require.Equal(t, " X;\n", problemLine(position)[64:])

		children := position.Children()
		require.NotEmpty(t, children)
		position = children[0]
	}
}
