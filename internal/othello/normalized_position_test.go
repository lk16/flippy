package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// rotatePosition applies spatial symmetry r to a position's discs directly, to check that Normalize
// collapses all 8 symmetries to the same NormalizedPosition.
func rotatePosition(b Position, r int) Position {
	return Position{
		player:   rotateBits(b.player, r),
		opponent: rotateBits(b.opponent, r),
	}
}

func testPositions(t *testing.T) []Position {
	t.Helper()

	positions := []Position{NewStartPosition()}

	position := NewStartPosition()
	for _, move := range []int{19, 18, 17, 9, 1, 0} {
		var err error
		position, err = position.DoMove(move)
		require.NoError(t, err)
		positions = append(positions, position)
	}

	return positions
}

func TestPosition_Normalize_Symmetry(t *testing.T) {
	for _, position := range testPositions(t) {
		want := position.Normalize()

		for r := range 8 {
			rotated := rotatePosition(position, r)
			require.Equal(t, want, rotated.Normalize(), "rotation %d should normalize the same way", r)
		}
	}
}

func TestPosition_Normalize_Idempotent(t *testing.T) {
	for _, position := range testPositions(t) {
		nb := position.Normalize()
		require.True(t, nb.Position().IsNormalized())
		require.Equal(t, nb, nb.Position().Normalize())
	}
}

func TestPosition_Normalize_PreservesDiscCount(t *testing.T) {
	for _, position := range testPositions(t) {
		require.Equal(t, position.CountDiscs(), position.Normalize().CountDiscs())
	}
}

func TestNewNormalizedPosition(t *testing.T) {
	position := testPositions(t)[len(testPositions(t))-1]
	rotated := rotatePosition(position, 1)
	require.NotEqual(t, position, rotated, "test position must not be symmetric for this test to be meaningful")

	_, err := NewNormalizedPosition(rotated)
	require.Error(t, err, "an arbitrary rotation should not already be in canonical form")

	nb, err := NewNormalizedPosition(rotated.Normalize().Position())
	require.NoError(t, err)
	require.Equal(t, rotated.Normalize(), nb)
}

func TestNormalizedPosition_Accessors(t *testing.T) {
	nb := NewStartPosition().Normalize()

	require.Equal(t, nb.Position().CountDiscs(), nb.CountDiscs())
	require.Equal(t, nb.Position().HasMoves(), nb.HasMoves())
	require.Equal(t, nb.Position().String(), nb.String())
}
