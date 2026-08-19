package othello

import "fmt"

// NormalizedBoard wraps a Board in canonical spatial form (see Board.Normalize).
type NormalizedBoard struct {
	board Board
}

// NewNormalizedBoard wraps board, which must already be in canonical form (see Board.Normalize).
func NewNormalizedBoard(board Board) (NormalizedBoard, error) {
	if !board.IsNormalized() {
		return NormalizedBoard{}, fmt.Errorf("board is not normalized: %s", board)
	}
	return NormalizedBoard{board: board}, nil
}

// Board returns the underlying board.
func (nb NormalizedBoard) Board() Board {
	return nb.board
}

// CountDiscs returns the total number of discs on the board.
func (nb NormalizedBoard) CountDiscs() int {
	return nb.board.CountDiscs()
}

// HasMoves reports whether the player to move has any legal move.
func (nb NormalizedBoard) HasMoves() bool {
	return nb.board.HasMoves()
}

// String returns the underlying board's textual encoding.
func (nb NormalizedBoard) String() string {
	return nb.board.String()
}
