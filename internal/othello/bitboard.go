package othello

const (
	boardWidth  = 8
	boardHeight = 8
	squareCount = boardWidth * boardHeight
)

// flipHorizontally mirrors a bitboard left-right.
func flipHorizontally(x uint64) uint64 {
	const (
		k1 = uint64(0x5555555555555555)
		k2 = uint64(0x3333333333333333)
		k4 = uint64(0x0F0F0F0F0F0F0F0F)
	)

	x = ((x >> 1) & k1) | ((x & k1) << 1)
	x = ((x >> 2) & k2) | ((x & k2) << 2)
	x = ((x >> 4) & k4) | ((x & k4) << 4)
	return x
}

// flipVertically mirrors a bitboard top-bottom.
func flipVertically(x uint64) uint64 {
	const (
		k1 = uint64(0x00FF00FF00FF00FF)
		k2 = uint64(0x0000FFFF0000FFFF)
	)

	x = ((x >> 8) & k1) | ((x & k1) << 8)
	x = ((x >> 16) & k2) | ((x & k2) << 16)
	x = (x >> 32) | (x << 32)
	return x
}

// flipDiagonally mirrors a bitboard across the a1-h8 diagonal.
func flipDiagonally(x uint64) uint64 {
	const (
		k1 = uint64(0x5500550055005500)
		k2 = uint64(0x3333000033330000)
		k4 = uint64(0x0F0F0F0F00000000)
	)

	t := k4 & (x ^ (x << 28))
	x ^= t ^ (t >> 28)
	t = k2 & (x ^ (x << 14))
	x ^= t ^ (t >> 14)
	t = k1 & (x ^ (x << 7))
	x ^= t ^ (t >> 7)
	return x
}

// rotateBits applies one of the 8 symmetries of the square to a bitboard,
// selected by combining the three flips as bit flags.
func rotateBits(x uint64, rotation int) uint64 {
	if rotation&1 != 0 {
		x = flipHorizontally(x)
	}
	if rotation&2 != 0 {
		x = flipVertically(x)
	}
	if rotation&4 != 0 {
		x = flipDiagonally(x)
	}
	return x
}

// legalMoves returns a bitboard of squares the mover can legally play on.
// Adapted from Edax.
func legalMoves(mover, opponent uint64) uint64 {
	const middleColumns = 0x7E7E7E7E7E7E7E7E

	mask := opponent & middleColumns

	flipL := mask & (mover << 1)
	flipL |= mask & (flipL << 1)
	maskL := mask & (mask << 1)
	flipL |= maskL & (flipL << 2)
	flipL |= maskL & (flipL << 2)
	flipR := mask & (mover >> 1)
	flipR |= mask & (flipR >> 1)
	maskR := mask & (mask >> 1)
	flipR |= maskR & (flipR >> 2)
	flipR |= maskR & (flipR >> 2)
	moves := (flipL << 1) | (flipR >> 1)

	flipL = mask & (mover << 7)
	flipL |= mask & (flipL << 7)
	maskL = mask & (mask << 7)
	flipL |= maskL & (flipL << 14)
	flipL |= maskL & (flipL << 14)
	flipR = mask & (mover >> 7)
	flipR |= mask & (flipR >> 7)
	maskR = mask & (mask >> 7)
	flipR |= maskR & (flipR >> 14)
	flipR |= maskR & (flipR >> 14)
	moves |= (flipL << 7) | (flipR >> 7)

	flipL = mask & (mover << 9)
	flipL |= mask & (flipL << 9)
	maskL = mask & (mask << 9)
	flipL |= maskL & (flipL << 18)
	flipL |= maskL & (flipL << 18)
	flipR = mask & (mover >> 9)
	flipR |= mask & (flipR >> 9)
	maskR = mask & (mask >> 9)
	flipR |= maskR & (flipR >> 18)
	flipR |= maskR & (flipR >> 18)
	moves |= (flipL << 9) | (flipR >> 9)

	flipL = opponent & (mover << 8)
	flipL |= opponent & (flipL << 8)
	maskL = opponent & (opponent << 8)
	flipL |= maskL & (flipL << 16)
	flipL |= maskL & (flipL << 16)
	flipR = opponent & (mover >> 8)
	flipR |= opponent & (flipR >> 8)
	maskR = opponent & (opponent >> 8)
	flipR |= maskR & (flipR >> 16)
	flipR |= maskR & (flipR >> 16)
	moves |= (flipL << 8) | (flipR >> 8)

	return moves &^ (mover | opponent)
}

// flippedDiscs returns the opponent discs that would flip if mover played on move (must be legal).
func flippedDiscs(mover, opponent uint64, move int) uint64 {
	directions := [8][2]int{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	var flipped uint64

	for _, dir := range directions {
		dx, dy := dir[0], dir[1]
		s := 1
		for {
			curx := (move % boardWidth) + (dx * s)
			cury := (move / boardWidth) + (dy * s)
			if curx < 0 || curx >= boardWidth || cury < 0 || cury >= boardHeight {
				break
			}

			cur := boardWidth*cury + curx
			curBit := uint64(1) << cur

			if opponent&curBit != 0 {
				s++
				continue
			}

			if mover&curBit != 0 && s >= 2 {
				for dist := 1; dist < s; dist++ {
					f := move + dist*(boardWidth*dy+dx)
					flipped |= uint64(1) << f
				}
			}
			break
		}
	}

	return flipped
}

// applyMove plays move (must be legal) and returns the resulting (mover, opponent) bitboards for the next player.
func applyMove(mover, opponent uint64, move int) (newMover, newOpponent uint64) {
	flipped := flippedDiscs(mover, opponent, move)
	moveBit := uint64(1) << move

	newOpponent = mover | flipped | moveBit
	newMover = opponent &^ newOpponent

	return newMover, newOpponent
}
