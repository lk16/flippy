package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrecomputedPositions12(t *testing.T) {
	positions := PrecomputedPositions12()

	require.Len(t, positions, 67245)

	seen := make(map[string]bool, len(positions))
	for _, position := range positions {
		require.Equal(t, 12, position.CountDiscs())
		require.True(t, position.Position().IsNormalized())
		require.True(t, position.HasMoves(), "position with no legal move should never be db-savable")

		key := position.String()
		require.False(t, seen[key], "duplicate position: %s", key)
		seen[key] = true
	}
}

func TestPrecomputedPositions12_ReturnsCopy(t *testing.T) {
	positions := PrecomputedPositions12()
	positions[0] = NormalizedPosition{}

	require.NotEqual(t, NormalizedPosition{}, PrecomputedPositions12()[0])
}

func TestParseNormalizedPositions_InvalidLine(t *testing.T) {
	_, err := parseNormalizedPositions("not-a-position")
	require.Error(t, err)
}

func TestParseNormalizedPositions_NotNormalized(t *testing.T) {
	position, err := NewStartPosition().DoMove(19)
	require.NoError(t, err)
	require.False(t, position.IsNormalized())

	_, err = parseNormalizedPositions(position.String())
	require.Error(t, err)
}
