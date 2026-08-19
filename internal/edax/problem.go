package edax

import "github.com/lk16/flippy/internal/othello"

// problemLine encodes board in edax's -solve format: 64 X/O/- squares, a space, and the color to
// move. edax's board_set reads X as the player to move and O as the opponent, which is exactly what
// othello.Board stores, so the color to move is always X.
func problemLine(board othello.Board) string {
	squares := make([]byte, 64)
	for i := range squares {
		mask := uint64(1) << i
		switch {
		case board.Player()&mask != 0:
			squares[i] = 'X'
		case board.Opponent()&mask != 0:
			squares[i] = 'O'
		default:
			squares[i] = '-'
		}
	}

	return string(squares) + " X;\n"
}
