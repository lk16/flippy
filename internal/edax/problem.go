package edax

import "github.com/lk16/flippy/internal/othello"

// problemLine encodes board in edax's -solve problem-file format: 64
// characters (black discs as 'X', white discs as 'O', empty squares as
// '-'), a space, then "X" or "O" naming the actual color to move, then
// ";\n". The color labels are fixed to the board's real colors — edax
// evaluates a position differently depending on which color is to move, so
// board.Turn() must be passed through as-is rather than always claiming X.
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
