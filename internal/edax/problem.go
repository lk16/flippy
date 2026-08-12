package edax

import "github.com/lk16/flippy/internal/othello"

// problemLine encodes board in edax's -solve format: 64 X/O/- squares, a space, and the real color to
// move ("X"/"O"). The turn field is what splits the squares into edax's (player, opponent) pair —
// board_set reads X into player, then swaps the two when the turn says O — so always claiming X would
// hand edax the position with the sides reversed. The color itself never reaches the search: it only
// sets the case of the printed PV, which is why a board and its color-flip score identically.
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
