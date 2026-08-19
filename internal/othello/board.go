package othello

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
)

// BoardBytesLength is the length in bytes of the encoding produced by Board.Bytes.
const BoardBytesLength = 16

// BoardStringLength is the length of the encoding produced by Board.String.
const BoardStringLength = 32

// PassMove is the sentinel move value representing a pass.
const PassMove = -1

const (
	startPlayerDiscs   = 0x0000000810000000
	startOpponentDiscs = 0x0000001008000000
)

// Board is an Othello position seen from the player to move: which squares hold that player's
// discs, and which hold the opponent's. Edax evaluates a position from those two bitboards alone,
// so whether the player to move is black or white is not part of a position and isn't stored; disc
// colors are a display concern of whatever shows a game.
type Board struct {
	player   uint64
	opponent uint64
}

// NewBoardStart returns a board set up with the standard Othello starting position.
func NewBoardStart() Board {
	return Board{
		player:   startPlayerDiscs,
		opponent: startOpponentDiscs,
	}
}

// NewBoard returns a board with the given discs, or an error if a square is claimed by both.
func NewBoard(player, opponent uint64) (Board, error) {
	if player&opponent != 0 {
		return Board{}, fmt.Errorf("player and opponent discs overlap: %#x", player&opponent)
	}

	return Board{player: player, opponent: opponent}, nil
}

// Player returns the bitboard of the discs of the player to move.
func (b Board) Player() uint64 {
	return b.player
}

// Opponent returns the bitboard of the discs of the player not to move.
func (b Board) Opponent() uint64 {
	return b.opponent
}

// CountDiscs returns the total number of discs on the board.
func (b Board) CountDiscs() int {
	return bits.OnesCount64(b.player | b.opponent)
}

// Moves returns a bitboard of the squares the player to move can legally play on.
func (b Board) Moves() uint64 {
	return legalMoves(b.player, b.opponent)
}

// HasMoves reports whether the player to move has any legal move.
func (b Board) HasMoves() bool {
	return b.Moves() != 0
}

// IsValidMove reports whether move is legal; PassMove is only legal with no other legal move.
func (b Board) IsValidMove(move int) bool {
	if move == PassMove {
		return !b.HasMoves()
	}
	if move < 0 || move >= squareCount {
		return false
	}
	return b.Moves()&(uint64(1)<<move) != 0
}

// DoMove plays move and returns the resulting board, or an error if move is not legal.
func (b Board) DoMove(move int) (Board, error) {
	if !b.IsValidMove(move) {
		return Board{}, fmt.Errorf("invalid move: %d", move)
	}

	if move == PassMove {
		return Board{player: b.opponent, opponent: b.player}, nil
	}

	player, opponent := applyMove(b.player, b.opponent, move)
	return Board{player: player, opponent: opponent}, nil
}

// Children returns the boards resulting from every legal move; does not include a pass.
func (b Board) Children() []Board {
	moves := b.Moves()
	children := make([]Board, 0, bits.OnesCount64(moves))

	for move := range squareCount {
		if moves&(uint64(1)<<move) == 0 {
			continue
		}

		if child, err := b.DoMove(move); err == nil {
			children = append(children, child)
		}
	}

	return children
}

// FinalScore returns the score of the player to move: positive if ahead, negative if behind, zero if tied.
func (b Board) FinalScore() int {
	playerCount := bits.OnesCount64(b.player)
	opponentCount := bits.OnesCount64(b.opponent)

	switch {
	case playerCount > opponentCount:
		return 64 - 2*opponentCount
	case opponentCount > playerCount:
		return -64 + 2*playerCount
	default:
		return 0
	}
}

// less orders boards by player discs, then opponent discs; Normalize picks the smallest symmetry.
func (b Board) less(other Board) bool {
	if b.player != other.player {
		return b.player < other.player
	}
	return b.opponent < other.opponent
}

// Normalize returns the canonical NormalizedBoard for b: the symmetry that sorts lowest.
func (b Board) Normalize() NormalizedBoard {
	best := b

	for r := 1; r < 8; r++ {
		rotated := Board{
			player:   rotateBits(b.player, r),
			opponent: rotateBits(b.opponent, r),
		}

		if rotated.less(best) {
			best = rotated
		}
	}

	return NormalizedBoard{board: best}
}

// IsNormalized reports whether b is already in its own canonical form.
func (b Board) IsNormalized() bool {
	return b.Normalize().Board() == b
}

// String returns a textual encoding: 16 hex digits of player discs, then 16 of opponent discs.
func (b Board) String() string {
	return fmt.Sprintf("%016x%016x", b.player, b.opponent)
}

// ParseBoard parses the format produced by Board.String().
func ParseBoard(s string) (Board, error) {
	if len(s) != BoardStringLength {
		return Board{}, fmt.Errorf("invalid board string %q: want length %d, got %d", s, BoardStringLength, len(s))
	}

	player, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return Board{}, fmt.Errorf("invalid board string %q: bad player discs: %w", s, err)
	}

	opponent, err := strconv.ParseUint(s[16:], 16, 64)
	if err != nil {
		return Board{}, fmt.Errorf("invalid board string %q: bad opponent discs: %w", s, err)
	}

	return NewBoard(player, opponent)
}

// Bytes returns the binary encoding of b: 8 bytes player discs, then 8 bytes opponent discs, both
// big-endian. This 128-bit value is what the boards table stores.
func (b Board) Bytes() []byte {
	buf := make([]byte, BoardBytesLength)
	binary.BigEndian.PutUint64(buf[0:8], b.player)
	binary.BigEndian.PutUint64(buf[8:16], b.opponent)
	return buf
}

// ParseBoardBytes parses the format produced by Board.Bytes.
func ParseBoardBytes(buf []byte) (Board, error) {
	if len(buf) != BoardBytesLength {
		return Board{}, fmt.Errorf("invalid board bytes: want length %d, got %d", BoardBytesLength, len(buf))
	}

	return NewBoard(binary.BigEndian.Uint64(buf[0:8]), binary.BigEndian.Uint64(buf[8:16]))
}
