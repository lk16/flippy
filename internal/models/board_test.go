package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBoardStart(t *testing.T) {
	board := NewBoardStart()

	// Check that it's a valid starting position
	require.Equal(t, BLACK, board.turn)
	require.Equal(t, NewPositionStart(), board.position)

	// Starting position should have 4 discs
	require.Equal(t, 4, board.position.CountDiscs())

	// Should have valid moves
	require.True(t, board.position.HasMoves())
}

func TestBoard_Position(t *testing.T) {
	board := NewBoardStart()
	position := board.Position()

	require.Equal(t, NewPositionStart(), position)
}

func TestBoard_IsValidMove(t *testing.T) {
	board := NewBoardStart()

	// Test valid moves in starting position
	validMoves := []int{19, 26, 37, 44} // c3, d3, e3, f3
	for _, move := range validMoves {
		require.True(t, board.IsValidMove(move), "Move %d should be valid", move)
	}

	// Test invalid moves in starting position
	invalidMoves := []int{0, 7, 56, 63, 27, 28, 35, 36} // corners and center squares
	for _, move := range invalidMoves {
		require.False(t, board.IsValidMove(move), "Move %d should be invalid", move)
	}

	// Test pass move when no valid moves
	// Create a position with no valid moves
	noMovesPosition := NewPositionMust(0xFFFFFFFFFFFFFFFF, 0x0000000000000000)
	noMovesBoard := Board{
		position: noMovesPosition,
		turn:     BLACK,
	}
	require.True(t, noMovesBoard.IsValidMove(PassMove))
	require.False(t, noMovesBoard.IsValidMove(0))
}

func TestBoard_opponent(t *testing.T) {
	board := NewBoardStart()

	// Black's opponent should be White
	require.Equal(t, WHITE, board.opponent())

	// Change turn to White
	board.turn = WHITE
	require.Equal(t, BLACK, board.opponent())
}

func TestBoard_DoMove(t *testing.T) {
	board := NewBoardStart()

	// Test a valid move
	move := 19 // c3
	newBoard := board.DoMove(move)

	// Turn should change
	require.Equal(t, WHITE, newBoard.turn)

	// Position should be different
	require.NotEqual(t, board.position, newBoard.position)

	// Test pass move
	passBoard := board.DoMove(PassMove)
	require.Equal(t, WHITE, passBoard.turn)
	// Position should be swapped (pass move)
	require.Equal(t, board.position.Opponent(), passBoard.position.Player())
	require.Equal(t, board.position.Player(), passBoard.position.Opponent())
}

func TestBoard_DoMove_InvalidMove(t *testing.T) {
	board := NewBoardStart()

	// Test invalid move (should return same board)
	invalidMove := 0 // corner
	newBoard := board.DoMove(invalidMove)

	// Should return the same board for invalid moves
	require.Equal(t, board.position, newBoard.position)
	require.Equal(t, board.turn, newBoard.turn)
}

func TestBoard_GetChildren(t *testing.T) {
	board := NewBoardStart()

	children := board.GetChildren()

	// Starting position should have 4 valid moves
	require.Len(t, children, 4)

	// All children should have different turn
	for _, child := range children {
		require.Equal(t, WHITE, child.turn)
	}

	// All children should have different positions
	positions := make(map[Position]bool)
	for _, child := range children {
		positions[child.position] = true
	}
	require.Len(t, positions, 4) // All positions should be unique
}

func TestBoard_GetChildren_NoMoves(t *testing.T) {
	// Create a position with no valid moves
	noMovesPosition := NewPositionMust(0xFFFFFFFFFFFFFFFF, 0x0000000000000000)
	noMovesBoard := Board{
		position: noMovesPosition,
		turn:     BLACK,
	}

	children := noMovesBoard.GetChildren()

	// Should have no children when no valid moves
	require.Empty(t, children)
}

func TestBoard_GetChildPositions(t *testing.T) {
	board := NewBoardStart()

	positions := board.GetChildPositions()

	// Should have same number of positions as children
	children := board.GetChildren()
	require.Len(t, positions, len(children))

	// All positions should be unique
	positionMap := make(map[Position]bool)
	for _, pos := range positions {
		positionMap[pos] = true
	}
	require.Len(t, positionMap, len(positions))
}

func TestBoard_Equal(t *testing.T) {
	board1 := NewBoardStart()
	board2 := NewBoardStart()

	// Same boards should be equal
	require.True(t, board1.Equal(board2))

	// Different turn should make boards unequal
	board2.turn = WHITE
	require.False(t, board1.Equal(board2))

	// Different position should make boards unequal
	board2.turn = BLACK
	board2.position = NewPositionEmpty()
	require.False(t, board1.Equal(board2))
}

func TestBoard_GameFlow(t *testing.T) {
	// Test a simple game flow
	board := NewBoardStart()

	// First move: c3
	board = board.DoMove(19) // c3
	require.Equal(t, WHITE, board.turn)

	// Second move: d3
	board = board.DoMove(20) // d3
	require.Equal(t, BLACK, board.turn)

	// Third move: e3
	board = board.DoMove(21) // e3
	require.Equal(t, WHITE, board.turn)

	// Check that the game state is progressing
	require.Greater(t, board.position.CountDiscs(), 4)
}

func TestBoard_Constants(t *testing.T) {
	// Test that constants are correctly defined
	require.Equal(t, BLACK, -1)
	require.Equal(t, 1, WHITE)
	require.Equal(t, 0, EMPTY)
	require.Equal(t, PassMove, -1)
}

func TestBoard_OpponentCalculation(t *testing.T) {
	// Test the opponent calculation formula: BLACK + WHITE - turn
	require.Equal(t, WHITE, BLACK+WHITE-BLACK)
	require.Equal(t, BLACK, BLACK+WHITE-WHITE)
}

func TestBoard_GetChildren_Consistency(t *testing.T) {
	board := NewBoardStart()

	// Get children directly
	children := board.GetChildren()

	// Get children through positions
	positions := board.position.GetChildren()
	expectedChildren := make([]Board, len(positions))
	for i, pos := range positions {
		expectedChildren[i] = Board{
			position: pos,
			turn:     board.opponent(),
		}
	}

	// Should be the same
	require.Len(t, children, len(expectedChildren))
	for i := range children {
		require.True(t, children[i].Equal(expectedChildren[i]))
	}
}
