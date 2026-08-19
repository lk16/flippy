package othello

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
)

// PositionBytesLength is the length in bytes of the encoding produced by Position.Bytes.
const PositionBytesLength = 16

// PositionStringLength is the length of the encoding produced by Position.String.
const PositionStringLength = 32

// PassMove is the sentinel move value representing a pass.
const PassMove = -1

const (
	startPlayerDiscs   = 0x0000000810000000
	startOpponentDiscs = 0x0000001008000000
)

// Position is an Othello position seen from the player to move: which squares hold that player's
// discs, and which hold the opponent's. Edax evaluates a position from those two bitboards alone,
// so whether the player to move is black or white is not part of a position and isn't stored; disc
// colors are a display concern of whatever shows a game.
type Position struct {
	player   uint64
	opponent uint64
}

// NewStartPosition returns the standard Othello starting position.
func NewStartPosition() Position {
	return Position{
		player:   startPlayerDiscs,
		opponent: startOpponentDiscs,
	}
}

// NewPosition returns a position with the given discs, or an error if a square is claimed by both.
func NewPosition(player, opponent uint64) (Position, error) {
	if player&opponent != 0 {
		return Position{}, fmt.Errorf("player and opponent discs overlap: %#x", player&opponent)
	}

	return Position{player: player, opponent: opponent}, nil
}

// Player returns the bitboard of the discs of the player to move.
func (p Position) Player() uint64 {
	return p.player
}

// Opponent returns the bitboard of the discs of the player not to move.
func (p Position) Opponent() uint64 {
	return p.opponent
}

// CountDiscs returns the total number of discs on the board.
func (p Position) CountDiscs() int {
	return bits.OnesCount64(p.player | p.opponent)
}

// Moves returns a bitboard of the squares the player to move can legally play on.
func (p Position) Moves() uint64 {
	return legalMoves(p.player, p.opponent)
}

// HasMoves reports whether the player to move has any legal move.
func (p Position) HasMoves() bool {
	return p.Moves() != 0
}

// IsValidMove reports whether move is legal; PassMove is only legal with no other legal move.
func (p Position) IsValidMove(move int) bool {
	if move == PassMove {
		return !p.HasMoves()
	}
	if move < 0 || move >= squareCount {
		return false
	}
	return p.Moves()&(uint64(1)<<move) != 0
}

// DoMove plays move and returns the resulting position, or an error if move is not legal.
func (p Position) DoMove(move int) (Position, error) {
	if !p.IsValidMove(move) {
		return Position{}, fmt.Errorf("invalid move: %d", move)
	}

	if move == PassMove {
		return Position{player: p.opponent, opponent: p.player}, nil
	}

	player, opponent := applyMove(p.player, p.opponent, move)
	return Position{player: player, opponent: opponent}, nil
}

// Children returns the positions resulting from every legal move; does not include a pass.
func (p Position) Children() []Position {
	moves := p.Moves()
	children := make([]Position, 0, bits.OnesCount64(moves))

	for move := range squareCount {
		if moves&(uint64(1)<<move) == 0 {
			continue
		}

		if child, err := p.DoMove(move); err == nil {
			children = append(children, child)
		}
	}

	return children
}

// FinalScore returns the score of the player to move: positive if ahead, negative if behind, zero if tied.
func (p Position) FinalScore() int {
	playerCount := bits.OnesCount64(p.player)
	opponentCount := bits.OnesCount64(p.opponent)

	switch {
	case playerCount > opponentCount:
		return 64 - 2*opponentCount
	case opponentCount > playerCount:
		return -64 + 2*playerCount
	default:
		return 0
	}
}

// less orders positions by player discs, then opponent discs; Normalize picks the smallest symmetry.
func (p Position) less(other Position) bool {
	if p.player != other.player {
		return p.player < other.player
	}
	return p.opponent < other.opponent
}

// Normalize returns the canonical NormalizedPosition for p: the symmetry that sorts lowest.
func (p Position) Normalize() NormalizedPosition {
	best := p

	for r := 1; r < 8; r++ {
		rotated := Position{
			player:   rotateBits(p.player, r),
			opponent: rotateBits(p.opponent, r),
		}

		if rotated.less(best) {
			best = rotated
		}
	}

	return NormalizedPosition{position: best}
}

// IsNormalized reports whether p is already in its own canonical form.
func (p Position) IsNormalized() bool {
	return p.Normalize().Position() == p
}

// String returns a textual encoding: 16 hex digits of player discs, then 16 of opponent discs.
func (p Position) String() string {
	return fmt.Sprintf("%016x%016x", p.player, p.opponent)
}

// ParsePosition parses the format produced by Position.String().
func ParsePosition(s string) (Position, error) {
	if len(s) != PositionStringLength {
		return Position{}, fmt.Errorf("invalid position string %q: want length %d, got %d", s, PositionStringLength, len(s))
	}

	player, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return Position{}, fmt.Errorf("invalid position string %q: bad player discs: %w", s, err)
	}

	opponent, err := strconv.ParseUint(s[16:], 16, 64)
	if err != nil {
		return Position{}, fmt.Errorf("invalid position string %q: bad opponent discs: %w", s, err)
	}

	return NewPosition(player, opponent)
}

// Bytes returns the binary encoding of p: 8 bytes player discs, then 8 bytes opponent discs, both
// big-endian. This 128-bit value is what the boards table stores.
func (p Position) Bytes() []byte {
	buf := make([]byte, PositionBytesLength)
	binary.BigEndian.PutUint64(buf[0:8], p.player)
	binary.BigEndian.PutUint64(buf[8:16], p.opponent)
	return buf
}

// ParsePositionBytes parses the format produced by Position.Bytes.
func ParsePositionBytes(buf []byte) (Position, error) {
	if len(buf) != PositionBytesLength {
		return Position{}, fmt.Errorf("invalid position bytes: want length %d, got %d", PositionBytesLength, len(buf))
	}

	return NewPosition(binary.BigEndian.Uint64(buf[0:8]), binary.BigEndian.Uint64(buf[8:16]))
}
