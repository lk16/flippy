package othello

// Color identifies a side in an Othello game.
type Color int

const (
	Black Color = iota
	White
)

// Opponent returns the other color.
func (c Color) Opponent() Color {
	return Black + White - c
}

// String returns "black" or "white".
func (c Color) String() string {
	if c == White {
		return "white"
	}
	return "black"
}
