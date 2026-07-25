package othello

import "fmt"

// NormalizedBoard wraps a Board that is in canonical spatial form (see
// Board.Normalize). The color to move is preserved rather than discarded.
type NormalizedBoard struct {
	board Board
}

// NewNormalizedBoard wraps board, which must already be in canonical form.
// Use Board.Normalize to construct a NormalizedBoard from an arbitrary
// board.
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

// Black returns the bitboard of black discs.
func (nb NormalizedBoard) Black() uint64 {
	return nb.board.Black()
}

// White returns the bitboard of white discs.
func (nb NormalizedBoard) White() uint64 {
	return nb.board.White()
}

// Turn returns the color to move.
func (nb NormalizedBoard) Turn() Color {
	return nb.board.Turn()
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
