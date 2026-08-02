package othello

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewGame(t *testing.T) {
	game := NewGame()

	require.Equal(t, NewBoardStart(), game.Board())
	require.Empty(t, game.Moves())
	require.Empty(t, game.Filename())
	require.Nil(t, game.Metadata())
}

func TestGame_PushMove(t *testing.T) {
	game := NewGame()

	require.NoError(t, game.PushMove(19))
	require.Equal(t, []int{19}, game.Moves())
	require.Equal(t, White, game.Board().Turn())

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
	// This sequence of otherwise-legal moves leads to a board where the
	// player to move (black) has no legal move but white does, so pushing
	// the final move should auto-insert a pass for black.
	moves := []int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2}

	game := NewGame()
	for _, m := range moves[:len(moves)-1] {
		require.NoError(t, game.PushMove(m))
	}
	require.True(t, game.Board().HasMoves())

	last := moves[len(moves)-1]
	require.NoError(t, game.PushMove(last))

	recorded := game.Moves()
	require.Equal(t, PassMove, recorded[len(recorded)-1], "a pass should have been auto-inserted")

	// The board right after the regular move (before the auto-inserted
	// pass) has no legal move for whoever's turn it is.
	require.False(t, game.BoardAt(len(recorded)-1).HasMoves())

	// After the pass, it's the other player's turn, who does have moves.
	require.True(t, game.Board().HasMoves())
}

func TestGame_PushMove_NoDoublePass(t *testing.T) {
	game := NewGame()
	game.moves = []int{19, PassMove}

	require.NoError(t, game.PushMove(PassMove))
	require.Equal(t, []int{19, PassMove}, game.Moves())
}

func TestGame_PushMove_ExplicitPass(t *testing.T) {
	// This sequence leads to a board where the player to move has no legal
	// move. The game is built directly from these boards (bypassing
	// PushMove) so that no pass has been auto-inserted yet, letting us test
	// that pushing PassMove directly (rather than relying on auto-insert
	// right after the regular move that necessitates it) is also accepted.
	moves := []int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2}

	boards := make([]Board, len(moves)+1)
	boards[0] = NewBoardStart()
	for i, m := range moves {
		var err error
		boards[i+1], err = boards[i].DoMove(m)
		require.NoError(t, err)
	}
	require.False(t, boards[len(boards)-1].HasMoves())

	game := &Game{moves: append([]int(nil), moves...), boards: boards}

	require.NoError(t, game.PushMove(PassMove))
	require.Equal(t, PassMove, game.Moves()[len(game.Moves())-1])
	require.True(t, game.Board().HasMoves())
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
	// A game that starts on a board with no legal move and whose only pushed
	// move is a pass: popping it must remove just that one pass, not try to
	// also remove a (nonexistent) preceding move and slice out of bounds.
	board, err := NewBoard(0xFFFFFFFFFFFFFFFF, 0, Black)
	require.NoError(t, err)

	game := NewGameWithStart(board)
	require.NoError(t, game.PushMove(PassMove))
	require.Equal(t, []int{PassMove}, game.Moves())

	require.NotPanics(t, game.PopMove)
	require.Empty(t, game.Moves())
	require.Equal(t, board, game.Board())
}

func TestGame_PopMove_RemovesTrailingPass(t *testing.T) {
	// PopMove's trailing-pass handling only looks at move values and slice
	// lengths, so the board cache content doesn't matter here as long as
	// its length tracks moves (one more entry, for the start board).
	game := &Game{
		moves:  []int{19, 18, PassMove},
		boards: make([]Board, 4),
	}

	game.PopMove()
	require.Equal(t, []int{19}, game.Moves())
}

func TestGame_BoardAt(t *testing.T) {
	game := NewGame()
	require.NoError(t, game.PushMove(19))
	require.NoError(t, game.PushMove(18))

	require.Equal(t, NewBoardStart(), game.BoardAt(0))
	require.Equal(t, 4, game.BoardAt(0).CountDiscs())
	require.Equal(t, 6, game.BoardAt(2).CountDiscs())
}

func TestGame_Boards(t *testing.T) {
	game := NewGame()
	require.NoError(t, game.PushMove(19))
	require.NoError(t, game.PushMove(18))

	boards := game.Boards()
	require.Len(t, boards, 3)
	require.Equal(t, game.BoardAt(0), boards[0])
	require.Equal(t, game.BoardAt(1), boards[1])
	require.Equal(t, game.BoardAt(2), boards[2])
}

func TestNewGameFromMoves(t *testing.T) {
	game, err := NewGameFromMoves([]int{19, 18, 17})
	require.NoError(t, err)
	require.Equal(t, []int{19, 18, 17}, game.Moves())

	_, err = NewGameFromMoves([]int{19, 0})
	require.Error(t, err)
}
