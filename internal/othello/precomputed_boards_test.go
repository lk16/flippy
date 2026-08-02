package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrecomputedBoards12(t *testing.T) {
	boards := PrecomputedBoards12()

	require.Len(t, boards, 67245)

	seen := make(map[string]bool, len(boards))
	for _, board := range boards {
		require.Equal(t, 12, board.CountDiscs())
		require.True(t, board.Board().IsNormalized())
		require.True(t, board.HasMoves(), "board with no legal move should never be db-savable")

		key := board.String()
		require.False(t, seen[key], "duplicate board: %s", key)
		seen[key] = true
	}
}

func TestPrecomputedBoards12_ReturnsCopy(t *testing.T) {
	boards := PrecomputedBoards12()
	boards[0] = NormalizedBoard{}

	require.NotEqual(t, NormalizedBoard{}, PrecomputedBoards12()[0])
}

func TestParseNormalizedBoards_InvalidLine(t *testing.T) {
	_, err := parseNormalizedBoards("not-a-board")
	require.Error(t, err)
}

func TestParseNormalizedBoards_NotNormalized(t *testing.T) {
	board, err := NewBoardStart().DoMove(19)
	require.NoError(t, err)
	require.False(t, board.IsNormalized())

	_, err = parseNormalizedBoards(board.String())
	require.Error(t, err)
}
