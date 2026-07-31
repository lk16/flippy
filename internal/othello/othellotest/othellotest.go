// Package othellotest provides board-construction helpers shared by the tests
// of several packages, so each doesn't have to redefine them. It lives in a
// non-test file so it can be imported across package boundaries, but is only
// referenced from _test.go files and so never ends up in a production binary.
package othellotest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// Board returns a NormalizedBoard reached by playing the first available legal
// move (or pass) from the starting position until it has exactly discs discs.
func Board(t *testing.T, discs int) othello.NormalizedBoard {
	t.Helper()

	board := othello.NewBoardStart()
	for board.CountDiscs() < discs {
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

	return board.Normalize()
}

// DistinctBoards returns n distinct NormalizedBoards with exactly discs discs,
// found via breadth-first search from the starting position.
func DistinctBoards(t *testing.T, discs, n int) []othello.NormalizedBoard {
	t.Helper()

	seen := make(map[othello.Board]bool)
	var result []othello.NormalizedBoard

	frontier := []othello.Board{othello.NewBoardStart()}
	for len(frontier) > 0 && len(result) < n {
		var next []othello.Board
		for _, board := range frontier {
			if board.CountDiscs() == discs {
				norm := board.Normalize()
				if key := norm.Board(); !seen[key] {
					seen[key] = true
					result = append(result, norm)
				}
				continue
			}

			if !board.HasMoves() {
				passed, err := board.DoMove(othello.PassMove)
				require.NoError(t, err)
				next = append(next, passed)
				continue
			}

			next = append(next, board.Children()...)
		}
		frontier = next
	}

	require.GreaterOrEqual(t, len(result), n)
	return result[:n]
}
