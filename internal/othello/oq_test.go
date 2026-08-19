package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOthelloQuestMoves(t *testing.T) {
	game, err := ParseOthelloQuestMoves("e6f4e3d6")
	require.NoError(t, err)
	require.Equal(t, []int{44, 29, 20, 43}, game.Moves())
}

func TestParseOthelloQuestMoves_AutoPass(t *testing.T) {
	// Field-string equivalent of the move sequence [19, 18, 17, 9, 1, 0, 37,
	// 43, 51, 2], which is known (see game_test.go) to reach a position with no
	// legal move for the player to move but one for their opponent, and so
	// should get a pass auto-inserted.
	game, err := ParseOthelloQuestMoves("d3c3b3b2b1a1f5d6d7c1")
	require.NoError(t, err)
	require.Equal(t, PassMove, game.Moves()[len(game.Moves())-1])
}

func TestParseOthelloQuestMoves_OddLength(t *testing.T) {
	_, err := ParseOthelloQuestMoves("e6f")
	require.Error(t, err)
}

func TestParseOthelloQuestMoves_InvalidField(t *testing.T) {
	_, err := ParseOthelloQuestMoves("z9")
	require.Error(t, err)
}

func TestParseOthelloQuestMoves_Empty(t *testing.T) {
	game, err := ParseOthelloQuestMoves("")
	require.NoError(t, err)
	require.Empty(t, game.Moves())
}
