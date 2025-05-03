package models

import (
	"fmt"
	"math/bits"
	"math/rand"
	"strings"
)

const (
	PassMove = -1
)

// flipHorizontally flips the bits of the position horizontally
func flipHorizontally(x uint64) uint64 {
	k1 := uint64(0x5555555555555555)
	k2 := uint64(0x3333333333333333)
	k4 := uint64(0x0F0F0F0F0F0F0F0F)
	x = ((x >> 1) & k1) | ((x & k1) << 1)
	x = ((x >> 2) & k2) | ((x & k2) << 2)
	x = ((x >> 4) & k4) | ((x & k4) << 4)
	return x & 0xFFFFFFFFFFFFFFFF
}

// flipVertically flips the bits of the position vertically
func flipVertically(x uint64) uint64 {
	k1 := uint64(0x00FF00FF00FF00FF)
	k2 := uint64(0x0000FFFF0000FFFF)

	x = ((x >> 8) & k1) | ((x & k1) << 8)
	x = ((x >> 16) & k2) | ((x & k2) << 16)
	x = (x >> 32) | (x << 32)
	return x & 0xFFFFFFFFFFFFFFFF
}

// flipDiagonally flips the bits of the position diagonally
func flipDiagonally(x uint64) uint64 {
	k1 := uint64(0x5500550055005500)
	k2 := uint64(0x3333000033330000)
	k4 := uint64(0x0F0F0F0F00000000)
	t := k4 & (x ^ (x << 28))
	x ^= t ^ (t >> 28)
	t = k2 & (x ^ (x << 14))
	x ^= t ^ (t >> 14)
	t = k1 & (x ^ (x << 7))
	x ^= t ^ (t >> 7)
	return x & 0xFFFFFFFFFFFFFFFF
}

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

// Position represents a position on the board
type Position struct {
	player   uint64 // Bitboard for the current player's pieces
	opponent uint64 // Bitboard for the opponent's pieces
}

// NewPosition creates a new position from a player and opponent bitboard
func NewPosition(player, opponent uint64) (Position, error) {
	if player&opponent != 0 {
		return Position{}, fmt.Errorf("invalid position: player and opponent discs cannot overlap")
	}

	return Position{
		player:   player,
		opponent: opponent,
	}, nil
}

// NewPositionMust creates a new position from a player and opponent bitboard
// and panics if the position is invalid
func NewPositionMust(player, opponent uint64) Position {
	p, err := NewPosition(player, opponent)
	if err != nil {
		panic(err)
	}
	return p
}

// NewPositionRandom creates a new position with a random number of discs
func NewPositionRandom(discs int) (Position, error) {
	if discs < 4 || discs > 64 {
		return Position{}, fmt.Errorf("invalid number of discs: %d", discs)
	}

	pos := NewPositionStart()

	for pos.CountDiscs() < discs {
		validMoves := pos.Moves()
		if validMoves == 0 {
			pos = NewPositionStart()
			continue
		}
		move := rand.Intn(64)
		if (1<<move)&validMoves != 0 {
			pos = pos.DoMove(move)
		}
	}

	return pos, nil
}

// NewPositionStart creates a new position with the starting position
func NewPositionStart() Position {
	return NewPositionMust(0x0000000810000000, 0x0000001008000000)
}

// NewPositionEmpty creates a new position with an empty board
func NewPositionEmpty() Position {
	return NewPositionMust(0, 0)
}

// Normalized returns the normalized position
func (p Position) Normalized() NormalizedPosition {
	normalized, _ := p.Normalize()
	return normalized
}

func (p Position) rotate(rotation int) Position {
	return Position{
		player:   rotateBits(p.player, rotation),
		opponent: rotateBits(p.opponent, rotation),
	}
}

func (p Position) unrotate(rotation int) Position {
	reverseRotation := []int{0, 1, 2, 3, 4, 6, 5, 7}
	return p.rotate(reverseRotation[rotation])
}

func (p Position) isLessThan(other Position) bool {
	if p.player < other.player {
		return true
	}
	if p.player == other.player && p.opponent < other.opponent {
		return true
	}
	return false
}

// Normalize normalizes the position
func (p Position) Normalize() (NormalizedPosition, int) {
	minPosition := p
	rotation := 0

	for r := 1; r < 8; r++ {
		rotated := p.rotate(r)

		if rotated.isLessThan(minPosition) {
			minPosition = rotated
			rotation = r
		}
	}

	// We don't use NewNormalizedPositionMust here, to prevent infinite recursion
	nPos := NormalizedPosition{
		position: minPosition,
	}

	return nPos, rotation
}

// IsNormalized checks if the position is normalized
func (p Position) IsNormalized() bool {
	return p.Normalized().Position() == p
}

// Player returns the player bitboard
func (p Position) Player() uint64 {
	return p.player
}

// Opponent returns the opponent bitboard
func (p Position) Opponent() uint64 {
	return p.opponent
}

// CountDiscs returns the number of discs on the board
func (p Position) CountDiscs() int {
	return bits.OnesCount64(p.player | p.opponent)
}

// HasMoves returns whether the position has any valid moves
func (p Position) HasMoves() bool {
	return p.Moves() != 0
}

// Moves returns a bitset with all valid moves for the player
// This code is adapted from Edax
func (p Position) Moves() uint64 {
	mask := p.opponent & 0x7E7E7E7E7E7E7E7E

	flipL := mask & (p.player << 1)
	flipL |= mask & (flipL << 1)
	maskL := mask & (mask << 1)
	flipL |= maskL & (flipL << (2 * 1))
	flipL |= maskL & (flipL << (2 * 1))
	flipR := mask & (p.player >> 1)
	flipR |= mask & (flipR >> 1)
	maskR := mask & (mask >> 1)
	flipR |= maskR & (flipR >> (2 * 1))
	flipR |= maskR & (flipR >> (2 * 1))
	movesSet := (flipL << 1) | (flipR >> 1)

	flipL = mask & (p.player << 7)
	flipL |= mask & (flipL << 7)
	maskL = mask & (mask << 7)
	flipL |= maskL & (flipL << (2 * 7))
	flipL |= maskL & (flipL << (2 * 7))
	flipR = mask & (p.player >> 7)
	flipR |= mask & (flipR >> 7)
	maskR = mask & (mask >> 7)
	flipR |= maskR & (flipR >> (2 * 7))
	flipR |= maskR & (flipR >> (2 * 7))
	movesSet |= (flipL << 7) | (flipR >> 7)

	flipL = mask & (p.player << 9)
	flipL |= mask & (flipL << 9)
	maskL = mask & (mask << 9)
	flipL |= maskL & (flipL << (2 * 9))
	flipL |= maskL & (flipL << (2 * 9))
	flipR = mask & (p.player >> 9)
	flipR |= mask & (flipR >> 9)
	maskR = mask & (mask >> 9)
	flipR |= maskR & (flipR >> (2 * 9))
	flipR |= maskR & (flipR >> (2 * 9))
	movesSet |= (flipL << 9) | (flipR >> 9)

	flipL = p.opponent & (p.player << 8)
	flipL |= p.opponent & (flipL << 8)
	maskL = p.opponent & (p.opponent << 8)
	flipL |= maskL & (flipL << (2 * 8))
	flipL |= maskL & (flipL << (2 * 8))
	flipR = p.opponent & (p.player >> 8)
	flipR |= p.opponent & (flipR >> 8)
	maskR = p.opponent & (p.opponent >> 8)
	flipR |= maskR & (flipR >> (2 * 8))
	flipR |= maskR & (flipR >> (2 * 8))
	movesSet |= (flipL << 8) | (flipR >> 8)

	movesSet &^= p.player | p.opponent
	return movesSet
}

// flipped returns a bitset with all the opponent discs that would be flipped if the player played on the given move
func (p Position) flipped(move int) uint64 {
	flipped := uint64(0)

	moveBit := uint64(1 << move)

	// If we try to play on an occupied square, this is an invalid move
	if (p.player|p.opponent)&moveBit != 0 {
		return 0
	}

	// Directions: horizontal, vertical, and both diagonals
	directions := [8][2]int{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	for _, dir := range directions {
		dx, dy := dir[0], dir[1]
		s := 1
		for {
			curx := (move % 8) + (dx * s)
			cury := (move / 8) + (dy * s)
			if curx < 0 || curx >= 8 || cury < 0 || cury >= 8 {
				break
			}

			cur := 8*cury + curx
			curBit := uint64(1 << cur)

			if p.opponent&curBit != 0 {
				s++
			} else {
				if (p.player&curBit != 0) && s >= 2 {
					for dist := 1; dist < s; dist++ {
						f := move + (dist * (8*dy + dx))
						flipped |= uint64(1 << f)
					}
				}
				break
			}
		}
	}

	return flipped
}

// DoMove does a move on the position.
func (p Position) DoMove(move int) Position {
	if move == PassMove {
		return Position{
			player:   p.opponent,
			opponent: p.player,
		}
	}

	moveBit := uint64(1 << move)

	// Check if the move is on an empty square
	if (p.player|p.opponent)&moveBit != 0 {
		return p
	}

	flipped := p.flipped(move)

	if flipped == 0 {
		return p
	}

	opp := p.player | flipped | moveBit
	me := p.opponent &^ opp

	return Position{
		player:   me,
		opponent: opp,
	}
}

// IsValidMove checks if a move is valid
func (p Position) IsValidMove(move int) bool {
	if move < PassMove || move >= 64 {
		return false
	}

	validMoves := p.Moves()

	if validMoves == 0 {
		return move == PassMove
	}

	return validMoves&(1<<move) != 0
}

// AsciiArtLines returns the ascii art lines for the position
func (p Position) AsciiArtLines() []string {
	moves := p.Moves()
	lines := make([]string, 10)

	lines[0] = "+-a-b-c-d-e-f-g-h-+"
	for y := 0; y < 8; y++ {
		line := fmt.Sprintf("%d ", y+1)

		for x := 0; x < 8; x++ {
			index := (y * 8) + x
			mask := uint64(1 << index)

			if p.player&mask != 0 {
				line += "○ "
			} else if p.opponent&mask != 0 {
				line += "● "
			} else if moves&mask != 0 {
				line += "· "
			} else {
				line += "  "
			}
		}

		lines[y+1] = line + "|"
	}

	lines[9] = "+-----------------+"

	return lines
}

// FieldToIndex converts a field notation (e.g. "a1", "h8") to an index (0-63)
// PassMove is returned if the field is "--", "ps", or "pa"
func FieldToIndex(field string) int {
	if len(field) != 2 {
		panic(fmt.Sprintf("invalid field length: %s", field))
	}

	field = strings.ToLower(field)

	if field == "--" || field == "ps" || field == "pa" {
		return PassMove
	}

	if !('a' <= field[0] && field[0] <= 'h' && '1' <= field[1] && field[1] <= '8') {
		panic(fmt.Sprintf("invalid field: %s", field))
	}

	x := int(field[0] - 'a')
	y := int(field[1] - '1')
	return y*8 + x
}
