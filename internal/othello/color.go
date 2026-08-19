package othello

// Color identifies a side in an Othello game. A Position has no color to move (see Position), so
// this is only used for the parts of a game that really are per-color: a PGN's players and winner.
type Color int

const (
	Black Color = iota
	White
)

// String returns "black" or "white".
func (c Color) String() string {
	if c == White {
		return "white"
	}
	return "black"
}

// newColor returns a pointer to c, for building optional-color fields.
func newColor(c Color) *Color {
	return &c
}
