package models

import (
	"fmt"
	"math/bits"
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

// Normalized returns the normalized position
func (p Position) Normalized() Position {
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

func (p Position) equals(other Position) bool {
	return p.player == other.player && p.opponent == other.opponent
}

// Normalize normalizes the position
func (p Position) Normalize() (Position, int) {
	minPosition := p
	rotation := 0

	for r := 1; r < 8; r++ {
		rotated := p.rotate(r)

		if rotated.isLessThan(minPosition) {
			minPosition = rotated
			rotation = r
		}
	}

	return minPosition, rotation
}

// IsNormalized checks if the position is normalized
func (p Position) IsNormalized() bool {
	return p.Normalized().equals(p)
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
