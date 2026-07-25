package othello

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
)

// BoardBytesLength is the length in bytes of the encoding produced by
// Board.Bytes.
const BoardBytesLength = 17

// PassMove is the sentinel move value representing a pass.
const PassMove = -1

const (
	startBlackDiscs = 0x0000000810000000
	startWhiteDiscs = 0x0000001008000000
)

// Board is an Othello position: which squares are black, which are white,
// and whose turn it is.
type Board struct {
	black uint64
	white uint64
	turn  Color
}

// NewBoardStart returns a board set up with the standard Othello starting
// position, black to move.
func NewBoardStart() Board {
	return Board{
		black: startBlackDiscs,
		white: startWhiteDiscs,
		turn:  Black,
	}
}

// NewBoardEmpty returns a board with no discs, black to move.
func NewBoardEmpty() Board {
	return Board{turn: Black}
}

// NewBoard returns a board with the given discs and turn. It returns an
// error if a square is claimed by both colors.
func NewBoard(black, white uint64, turn Color) (Board, error) {
	if black&white != 0 {
		return Board{}, fmt.Errorf("black and white discs overlap: %#x", black&white)
	}

	return Board{black: black, white: white, turn: turn}, nil
}

// Black returns the bitboard of black discs.
func (b Board) Black() uint64 {
	return b.black
}

// White returns the bitboard of white discs.
func (b Board) White() uint64 {
	return b.white
}

// Turn returns the color to move.
func (b Board) Turn() Color {
	return b.turn
}

// CountDiscs returns the total number of discs on the board.
func (b Board) CountDiscs() int {
	return bits.OnesCount64(b.black | b.white)
}

// mover returns the bitboard of the player to move.
func (b Board) mover() uint64 {
	if b.turn == Black {
		return b.black
	}
	return b.white
}

// opponent returns the bitboard of the player not to move.
func (b Board) opponent() uint64 {
	if b.turn == Black {
		return b.white
	}
	return b.black
}

// fromMoverOpponent rebuilds a board's black/white fields from bitboards
// expressed relative to the player to move.
func fromMoverOpponent(mover, opponent uint64, turn Color) Board {
	b := Board{turn: turn}
	if turn == Black {
		b.black, b.white = mover, opponent
	} else {
		b.white, b.black = mover, opponent
	}
	return b
}

// Moves returns a bitboard of the squares the player to move can legally
// play on.
func (b Board) Moves() uint64 {
	return legalMoves(b.mover(), b.opponent())
}

// HasMoves reports whether the player to move has any legal move.
func (b Board) HasMoves() bool {
	return b.Moves() != 0
}

// IsValidMove reports whether move is legal for the player to move. PassMove
// is only legal when the player to move has no other legal move.
func (b Board) IsValidMove(move int) bool {
	if move == PassMove {
		return !b.HasMoves()
	}
	if move < 0 || move >= squareCount {
		return false
	}
	return b.Moves()&(uint64(1)<<move) != 0
}

// DoMove plays move and returns the resulting board, with turn passed to the
// opponent. It returns an error if move is not legal.
func (b Board) DoMove(move int) (Board, error) {
	if !b.IsValidMove(move) {
		return Board{}, fmt.Errorf("invalid move: %d", move)
	}

	if move == PassMove {
		return fromMoverOpponent(b.opponent(), b.mover(), b.turn.Opponent()), nil
	}

	newMover, newOpponent := applyMove(b.mover(), b.opponent(), move)
	return fromMoverOpponent(newMover, newOpponent, b.turn.Opponent()), nil
}

// Children returns the boards resulting from every legal move of the player
// to move. It does not include a pass.
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

// FinalScore returns the score from the perspective of the player to move:
// positive if they have more discs, negative if fewer, zero if tied.
func (b Board) FinalScore() int {
	moverCount := bits.OnesCount64(b.mover())
	opponentCount := bits.OnesCount64(b.opponent())

	switch {
	case moverCount > opponentCount:
		return 64 - 2*opponentCount
	case opponentCount > moverCount:
		return -64 + 2*moverCount
	default:
		return 0
	}
}

// Normalize returns the canonical NormalizedBoard for b: among the 8
// symmetries (rotations/reflections) of the square, the one whose (mover,
// opponent) bitboard pair sorts lowest. Turn is carried through unchanged,
// rather than folded into the comparison.
func (b Board) Normalize() NormalizedBoard {
	bestMover, bestOpponent := b.mover(), b.opponent()

	for r := 1; r < 8; r++ {
		mover := rotateBits(b.mover(), r)
		opponent := rotateBits(b.opponent(), r)

		if mover < bestMover || (mover == bestMover && opponent < bestOpponent) {
			bestMover, bestOpponent = mover, opponent
		}
	}

	return NormalizedBoard{board: fromMoverOpponent(bestMover, bestOpponent, b.turn)}
}

// IsNormalized reports whether b is already in its own canonical form.
func (b Board) IsNormalized() bool {
	return b.Normalize().Board() == b
}

// String returns a textual encoding of the board: 16 hex digits of black
// discs, 16 hex digits of white discs, then "-b" or "-w" for the turn.
func (b Board) String() string {
	turnSuffix := "-b"
	if b.turn == White {
		turnSuffix = "-w"
	}
	return fmt.Sprintf("%016x%016x%s", b.black, b.white, turnSuffix)
}

// ParseBoard parses the format produced by Board.String(): 16 hex digits of
// black discs, 16 hex digits of white discs, then "-b" or "-w" for the turn.
func ParseBoard(s string) (Board, error) {
	if len(s) != 34 {
		return Board{}, fmt.Errorf("invalid board string %q: want length 34, got %d", s, len(s))
	}

	black, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return Board{}, fmt.Errorf("invalid board string %q: bad black discs: %w", s, err)
	}

	white, err := strconv.ParseUint(s[16:32], 16, 64)
	if err != nil {
		return Board{}, fmt.Errorf("invalid board string %q: bad white discs: %w", s, err)
	}

	var turn Color
	switch s[32:] {
	case "-b":
		turn = Black
	case "-w":
		turn = White
	default:
		return Board{}, fmt.Errorf("invalid board string %q: bad turn suffix", s)
	}

	return NewBoard(black, white, turn)
}

// Bytes returns the binary encoding of b: 8 bytes of black discs, 8 bytes of
// white discs (both big-endian), then 1 byte for the turn (0 = black, 1 =
// white). Suitable as a compact DB key.
func (b Board) Bytes() []byte {
	buf := make([]byte, BoardBytesLength)
	binary.BigEndian.PutUint64(buf[0:8], b.black)
	binary.BigEndian.PutUint64(buf[8:16], b.white)
	if b.turn == White {
		buf[16] = 1
	}
	return buf
}

// ParseBoardBytes parses the format produced by Board.Bytes.
func ParseBoardBytes(buf []byte) (Board, error) {
	if len(buf) != BoardBytesLength {
		return Board{}, fmt.Errorf("invalid board bytes: want length %d, got %d", BoardBytesLength, len(buf))
	}

	black := binary.BigEndian.Uint64(buf[0:8])
	white := binary.BigEndian.Uint64(buf[8:16])

	var turn Color
	switch buf[16] {
	case 0:
		turn = Black
	case 1:
		turn = White
	default:
		return Board{}, fmt.Errorf("invalid board bytes: bad turn byte %d", buf[16])
	}

	return NewBoard(black, white, turn)
}
