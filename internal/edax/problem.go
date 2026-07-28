package edax

import "github.com/lk16/flippy/internal/othello"

// problemLine encodes board in edax's -solve format: 64 X/O/- squares, a space, and the real color to
// move ("X"/"O") — edax evaluates differently per color to move, so this must not always claim X.
func problemLine(board othello.Board) string {
	squares := make([]byte, 64)
	for i := range squares {
		mask := uint64(1) << i
		switch {
		case board.Black()&mask != 0:
			squares[i] = 'X'
		case board.White()&mask != 0:
			squares[i] = 'O'
		default:
			squares[i] = '-'
		}
	}

	turn := "X"
	if board.Turn() == othello.White {
		turn = "O"
	}

	return string(squares) + " " + turn + ";\n"
}
