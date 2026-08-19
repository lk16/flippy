package edax

import "github.com/lk16/flippy/internal/othello"

// problemLine encodes position in edax's -solve format: 64 X/O/- squares, a space, and the color
// to move. edax's board_set reads X as the player to move and O as the opponent, which is exactly
// what othello.Position stores, so the color to move is always X.
func problemLine(position othello.Position) string {
	squares := make([]byte, 64)
	for i := range squares {
		mask := uint64(1) << i
		switch {
		case position.Player()&mask != 0:
			squares[i] = 'X'
		case position.Opponent()&mask != 0:
			squares[i] = 'O'
		default:
			squares[i] = '-'
		}
	}

	return string(squares) + " X;\n"
}
