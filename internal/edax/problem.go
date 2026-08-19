package edax

import "github.com/lk16/flippy/internal/othello"

// problemLine encodes board in edax's -solve format: 64 X/O/- squares, a space, and the real color
// to move ("X"/"O"). The turn must be the real color: edax's board_set swaps (player, opponent)
// when the turn says O, so always claiming X would hand edax the position with sides reversed.
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
