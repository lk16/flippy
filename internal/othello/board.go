package othello

import (
	"fmt"
	"strconv"
)

const (
	BLACK = 0
	WHITE = 1
	EMPTY = 2
	DRAW  = EMPTY
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

// NewBoardEmpty creates a new board with an empty position.
func NewBoardEmpty() Board {
	return Board{
		position: NewPositionEmpty(),
		turn:     BLACK,
	}
}

// NewBoardFromString creates a new board from a string representation.
func NewBoardFromString(s string) (Board, error) {
	if len(s) != 34 {
		return Board{}, fmt.Errorf("board string must be 34 characters long, got %d", len(s))
	}

	player, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return Board{}, fmt.Errorf("invalid player position: %w", err)
	}

	opponent, err := strconv.ParseUint(s[16:32], 16, 64)
	if err != nil {
		return Board{}, fmt.Errorf("invalid opponent position: %w", err)
	}

	var turn int
	switch s[32:34] {
	case "-w":
		turn = WHITE
	case "-b":
		turn = BLACK
	default:
		return Board{}, fmt.Errorf("invalid turn: %s", s[32:34])
	}

	board := Board{
		position: NewPositionMust(player, opponent),
		turn:     turn,
	}

	return board, nil
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
	// TODO return error on invalid move

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

// GetSquare returns the square at the given index.
func (b Board) GetSquare(index int) int {
	mask := uint64(1) << index
	if b.position.player&mask != 0 {
		if b.turn == WHITE {
			return WHITE
		}
		return BLACK
	}
	if b.position.opponent&mask != 0 {
		if b.turn == BLACK {
			return WHITE
		}
		return BLACK
	}
	return EMPTY
}

// Turn returns the turn.
func (b Board) Turn() int {
	return b.turn
}

// HasMoves checks if the board has moves.
func (b Board) HasMoves() bool {
	return b.position.HasMoves()
}

// GetNormalizedChildren returns all normalized children for a board.
func (b Board) GetNormalizedChildren() []NormalizedPosition {
	return b.position.GetNormalizedChildren()
}

// GetFinalScore returns the final score of the board.
func (b Board) GetFinalScore() int {
	return b.position.GetFinalScore()
}

// Moves returns the moves for the board.
func (b Board) Moves() uint64 {
	return b.position.Moves()
}

// ASCIIArtLines returns the ascii art lines for the position.
func (b Board) ASCIIArtLines() []string {
	moves := b.Moves()

	var black, white uint64

	if b.turn == WHITE {
		black = b.position.opponent
		white = b.position.player
	} else {
		black = b.position.player
		white = b.position.opponent
	}
	lines := make([]string, MaxY+2)

	lines[0] = "+-a-b-c-d-e-f-g-h-+"
	for y := range MaxY {
		line := fmt.Sprintf("%d ", y+1)

		for x := range MaxX {
			index := (y * MaxX) + x
			mask := uint64(1 << index)

			switch {
			case white&mask != 0:
				line += "○ "
			case black&mask != 0:
				line += "● "
			case moves&mask != 0:
				line += "· "
			default:
				line += "  "
			}
		}

		lines[y+1] = line + "|"
	}

	lines[9] = "+-----------------+"

	return lines
}

// Print prints the board to the console. This is used for debugging.
func (b Board) Print() {
	lines := b.ASCIIArtLines()
	for _, line := range lines {
		fmt.Println(line)
	}
}

// String returns the string representation of the board.
func (b Board) String() string {
	var turnString string
	if b.turn == WHITE {
		turnString = "-w"
	} else {
		turnString = "-b"
	}

	return fmt.Sprintf("%016x%016x%s", b.position.player, b.position.opponent, turnString)
}
