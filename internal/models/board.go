package models

const (
	BLACK = -1
	WHITE = 1
	EMPTY = 0
)

// Board represents an Othello board with position and turn information.
type Board struct {
	position Position
	turn     int
}

// NewBoardStart creates a new board with the starting position.
func NewBoardStart() Board {
	return Board{
		position: NewPositionStart(),
		turn:     BLACK,
	}
}

// Position returns the underlying position.
func (b Board) Position() Position {
	return b.position
}

// IsValidMove checks if a move is valid.
func (b Board) IsValidMove(move int) bool {
	return b.position.IsValidMove(move)
}

// opponent returns the opponent color.
func (b Board) opponent() int {
	return BLACK + WHITE - b.turn
}

// DoMove performs a move and returns the new board.
func (b Board) DoMove(move int) Board {
	position := b.position.DoMove(move)

	// If the move is invalid, return the same board
	if position == b.position {
		return b
	}

	turn := b.opponent()
	return Board{
		position: position,
		turn:     turn,
	}
}

// GetChildren returns all possible child boards.
func (b Board) GetChildren() []Board {
	positions := b.position.GetChildren()
	children := make([]Board, len(positions))
	for i, pos := range positions {
		children[i] = Board{
			position: pos,
			turn:     b.opponent(),
		}
	}
	return children
}

// GetChildPositions returns all possible child positions.
func (b Board) GetChildPositions() []Position {
	children := b.GetChildren()
	positions := make([]Position, len(children))
	for i, child := range children {
		positions[i] = child.Position()
	}
	return positions
}

// Equal checks if two boards are equal.
func (b Board) Equal(other Board) bool {
	return b.position == other.position && b.turn == other.turn
}
