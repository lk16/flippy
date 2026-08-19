package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewGame(t *testing.T) {
	game := NewGame()

	require.Equal(t, NewStartPosition(), game.Position())
	require.Empty(t, game.Moves())
	require.Empty(t, game.Filename())
	require.Nil(t, game.Metadata())
}

func TestGame_PushMove(t *testing.T) {
	game := NewGame()

	require.NoError(t, game.PushMove(19))
	require.Equal(t, []int{19}, game.Moves())

	require.NoError(t, game.PushMove(18))
	require.Equal(t, []int{19, 18}, game.Moves())
}

func TestGame_PushMove_Invalid(t *testing.T) {
	game := NewGame()

	err := game.PushMove(0)
	require.Error(t, err)
	require.Empty(t, game.Moves())
}

func TestGame_PushMove_AutoPass(t *testing.T) {
	// This sequence reaches a position where black (to move) has no legal move but white does, so
	// pushing the final move should auto-insert a pass.
	moves := []int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2}

	game := NewGame()
	for _, m := range moves[:len(moves)-1] {
		require.NoError(t, game.PushMove(m))
	}
	require.True(t, game.Position().HasMoves())

	last := moves[len(moves)-1]
	require.NoError(t, game.PushMove(last))

	recorded := game.Moves()
	require.Equal(t, PassMove, recorded[len(recorded)-1], "a pass should have been auto-inserted")

	// The position right after the regular move (before the pass) has no legal move.
	require.False(t, game.PositionAt(len(recorded)-1).HasMoves())

	// After the pass, it's the other player's turn, who does have moves.
	require.True(t, game.Position().HasMoves())
}

func TestGame_PushMove_NoDoublePass(t *testing.T) {
	game := NewGame()
	game.moves = []int{19, PassMove}

	require.NoError(t, game.PushMove(PassMove))
	require.Equal(t, []int{19, PassMove}, game.Moves())
}

func TestGame_PushMove_ExplicitPass(t *testing.T) {
	// The game is built directly from positions (bypassing PushMove) so no pass has been
	// auto-inserted yet, testing that pushing PassMove explicitly is also accepted.
	moves := []int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2}

	positions := make([]Position, len(moves)+1)
	positions[0] = NewStartPosition()
	for i, m := range moves {
		var err error
		positions[i+1], err = positions[i].DoMove(m)
		require.NoError(t, err)
	}
	require.False(t, positions[len(positions)-1].HasMoves())

	game := &Game{moves: append([]int(nil), moves...), positions: positions}

	require.NoError(t, game.PushMove(PassMove))
	require.Equal(t, PassMove, game.Moves()[len(game.Moves())-1])
	require.True(t, game.Position().HasMoves())
}

func TestGame_PopMove(t *testing.T) {
	game := NewGame()
	require.NoError(t, game.PushMove(19))
	require.NoError(t, game.PushMove(18))

	game.PopMove()
	require.Equal(t, []int{19}, game.Moves())

	game.PopMove()
	require.Empty(t, game.Moves())

	// Popping an empty game is a no-op.
	game.PopMove()
	require.Empty(t, game.Moves())
}

func TestGame_PopMove_LoneTrailingPass(t *testing.T) {
	// The only pushed move is a pass: popping must remove just that pass, not also a
	// (nonexistent) preceding move and slice out of bounds.
	position, err := NewPosition(0xFFFFFFFFFFFFFFFF, 0)
	require.NoError(t, err)

	game := NewGameWithStart(position)
	require.NoError(t, game.PushMove(PassMove))
	require.Equal(t, []int{PassMove}, game.Moves())

	require.NotPanics(t, game.PopMove)
	require.Empty(t, game.Moves())
	require.Equal(t, position, game.Position())
}

func TestGame_PopMove_RemovesTrailingPass(t *testing.T) {
	// PopMove only inspects move values and slice lengths, so the positions' content doesn't
	// matter as long as the slice length tracks moves.
	game := &Game{
		moves:     []int{19, 18, PassMove},
		positions: make([]Position, 4),
	}

	game.PopMove()
	require.Equal(t, []int{19}, game.Moves())
}

func TestGame_BoardAt(t *testing.T) {
	game := NewGame()
	require.NoError(t, game.PushMove(19))
	require.NoError(t, game.PushMove(18))

	require.Equal(t, NewStartPosition(), game.PositionAt(0))
	require.Equal(t, 4, game.PositionAt(0).CountDiscs())
	require.Equal(t, 6, game.PositionAt(2).CountDiscs())
}

func TestGame_Boards(t *testing.T) {
	game := NewGame()
	require.NoError(t, game.PushMove(19))
	require.NoError(t, game.PushMove(18))

	positions := game.Positions()
	require.Len(t, positions, 3)
	require.Equal(t, game.PositionAt(0), positions[0])
	require.Equal(t, game.PositionAt(1), positions[1])
	require.Equal(t, game.PositionAt(2), positions[2])
}

func TestNewGameFromMoves(t *testing.T) {
	game, err := NewGameFromMoves([]int{19, 18, 17})
	require.NoError(t, err)
	require.Equal(t, []int{19, 18, 17}, game.Moves())

	_, err = NewGameFromMoves([]int{19, 0})
	require.Error(t, err)
}
