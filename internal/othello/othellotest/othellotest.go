// Package othellotest provides position-construction helpers shared by several packages' tests; a
// non-test file so it's importable across packages, but only _test.go files reference it.
package othellotest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// Position returns a NormalizedPosition reached by playing the first available legal
// move (or pass) from the starting position until it has exactly discs discs.
func Position(t *testing.T, discs int) othello.NormalizedPosition {
	t.Helper()

	position := othello.NewStartPosition()
	for position.CountDiscs() < discs {
		if !position.HasMoves() {
			next, err := position.DoMove(othello.PassMove)
			require.NoError(t, err)
			position = next
			continue
		}

		children := position.Children()
		require.NotEmpty(t, children)
		position = children[0]
	}

	return position.Normalize()
}

// DistinctPositions returns n distinct NormalizedPositions with exactly discs discs,
// found via breadth-first search from the starting position.
func DistinctPositions(t *testing.T, discs, n int) []othello.NormalizedPosition {
	t.Helper()

	seen := make(map[othello.Position]bool)
	var result []othello.NormalizedPosition

	frontier := []othello.Position{othello.NewStartPosition()}
	for len(frontier) > 0 && len(result) < n {
		var next []othello.Position
		for _, position := range frontier {
			if position.CountDiscs() == discs {
				norm := position.Normalize()
				if key := norm.Position(); !seen[key] {
					seen[key] = true
					result = append(result, norm)
				}
				continue
			}

			if !position.HasMoves() {
				passed, err := position.DoMove(othello.PassMove)
				require.NoError(t, err)
				next = append(next, passed)
				continue
			}

			next = append(next, position.Children()...)
		}
		frontier = next
	}

	require.GreaterOrEqual(t, len(result), n)
	return result[:n]
}
