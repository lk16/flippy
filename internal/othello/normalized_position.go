package othello

import "fmt"

// NormalizedPosition wraps a Position in canonical spatial form (see Position.Normalize).
type NormalizedPosition struct {
	position Position
}

// NewNormalizedPosition wraps position, which must already be in canonical form (see Position.Normalize).
func NewNormalizedPosition(position Position) (NormalizedPosition, error) {
	if !position.IsNormalized() {
		return NormalizedPosition{}, fmt.Errorf("position is not normalized: %s", position)
	}
	return NormalizedPosition{position: position}, nil
}

// Position returns the underlying position.
func (np NormalizedPosition) Position() Position {
	return np.position
}

// CountDiscs returns the total number of discs on the board.
func (np NormalizedPosition) CountDiscs() int {
	return np.position.CountDiscs()
}

// HasMoves reports whether the player to move has any legal move.
func (np NormalizedPosition) HasMoves() bool {
	return np.position.HasMoves()
}

// String returns the underlying position's textual encoding.
func (np NormalizedPosition) String() string {
	return np.position.String()
}
