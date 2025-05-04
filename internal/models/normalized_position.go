package models

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/lk16/flippy/api/internal/config"
)

const (
	NormalizedPositionStringLength = 32
	NormalizedPositionBytesLength  = 16
)

// NormalizedPosition represents a normalized position on the board
// We make sure it's normalized by normalizing it when we create it.
type NormalizedPosition struct {
	position Position
}

// NewNormalizedPositionFromString creates a new normalized position from a string.
func NewNormalizedPositionFromString(s string) (NormalizedPosition, error) {
	if len(s) != NormalizedPositionStringLength {
		return NormalizedPosition{}, fmt.Errorf("position string must be exactly 32 characters, got %d", len(s))
	}

	player, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return NormalizedPosition{}, fmt.Errorf("invalid player position: %w", err)
	}

	opponent, err := strconv.ParseUint(s[16:], 16, 64)
	if err != nil {
		return NormalizedPosition{}, fmt.Errorf("invalid opponent position: %w", err)
	}

	return NewNormalizedPositionFromUint64s(player, opponent)
}

// NewNormalizedPositionFromBytes creates a new normalized position from a byte slice.
func NewNormalizedPositionFromBytes(b []byte) (NormalizedPosition, error) {
	if len(b) != NormalizedPositionBytesLength {
		return NormalizedPosition{}, fmt.Errorf("position bytes must be exactly 16 bytes, got %d", len(b))
	}

	player := binary.LittleEndian.Uint64(b[:8])
	opponent := binary.LittleEndian.Uint64(b[8:])

	return NewNormalizedPositionFromUint64s(player, opponent)
}

// NewNormalizedPositionEmpty creates a new normalized position with no discs.
func NewNormalizedPositionEmpty() NormalizedPosition {
	return NewNormalizedPositionMust(0, 0)
}

// NewNormalizedPositionFromUint64s creates a new normalized position from a player and opponent bitboard.
func NewNormalizedPositionFromUint64s(player, opponent uint64) (NormalizedPosition, error) {
	pos, err := NewPosition(player, opponent)
	if err != nil {
		return NormalizedPosition{}, fmt.Errorf("invalid normalized position: %w", err)
	}

	if !pos.IsNormalized() {
		return NormalizedPosition{}, errors.New("invalid normalized position: position is not normalized")
	}

	return NormalizedPosition{
		position: pos,
	}, nil
}

// NewNormalizedPositionMust creates a new normalized position from a player and opponent bitboard.
// It panics if the position is invalid.
func NewNormalizedPositionMust(player, opponent uint64) NormalizedPosition {
	nPos, err := NewNormalizedPositionFromUint64s(player, opponent)
	if err != nil {
		panic(fmt.Sprintf("invalid normalized position: %s", err))
	}
	return nPos
}

// String implements the Stringer interface for Position.
// It returns a 32-character hex string where the first 16 characters represent the player's pieces
// and the last 16 characters represent the opponent's pieces.
func (nPos *NormalizedPosition) String() string {
	return fmt.Sprintf("%016X%016X", nPos.Player(), nPos.Opponent())
}

// Bytes returns the normalized position as a byte slice.
func (nPos *NormalizedPosition) Bytes() []byte {
	b := make([]byte, NormalizedPositionBytesLength)
	binary.LittleEndian.PutUint64(b[:8], nPos.Player())
	binary.LittleEndian.PutUint64(b[8:], nPos.Opponent())
	return b
}

// UnmarshalJSON implements json.Unmarshaler for NormalizedPosition.
// It expects a 32-character hex string where the first 16 characters represent the player's pieces
// and the last 16 characters represent the opponent's pieces.
func (nPos *NormalizedPosition) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid position string: %w", err)
	}

	var err error
	*nPos, err = NewNormalizedPositionFromString(s)
	if err != nil {
		return fmt.Errorf("invalid position string: %w", err)
	}

	return nil
}

// MarshalJSON implements json.Marshaler for NormalizedPosition.
// It returns a 32-character hex string where the first 16 characters represent the player's pieces
// and the last 16 characters represent the opponent's pieces.
func (nPos NormalizedPosition) MarshalJSON() ([]byte, error) {
	return json.Marshal(nPos.String())
}

// Player returns the player bitboard.
func (nPos *NormalizedPosition) Player() uint64 {
	return nPos.position.Player()
}

// Opponent returns the opponent bitboard.
func (nPos *NormalizedPosition) Opponent() uint64 {
	return nPos.position.Opponent()
}

// Position returns the underlying position.
func (nPos *NormalizedPosition) Position() Position {
	return nPos.position
}

// CountDiscs returns the number of discs on the board.
func (nPos *NormalizedPosition) CountDiscs() int {
	return nPos.position.CountDiscs()
}

// Scan implements the sql.Scanner interface for NormalizedPosition.
func (nPos *NormalizedPosition) Scan(value interface{}) error {
	if value == nil {
		return errors.New("cannot scan nil into NormalizedPosition")
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into NormalizedPosition", value)
	}

	nPosFromBytes, err := NewNormalizedPositionFromBytes(bytes)
	if err != nil {
		return fmt.Errorf("error scanning position: %w", err)
	}

	*nPos = nPosFromBytes
	return nil
}

// IsDBSavable returns whether the position should be saved in the database.
func (nPos *NormalizedPosition) IsDBSavable() bool {
	discCount := nPos.CountDiscs()

	return discCount >= 4 && discCount <= config.MaxBookSavableDiscs && nPos.HasMoves()
}

// HasMoves returns whether the position has any valid moves.
func (nPos *NormalizedPosition) HasMoves() bool {
	return nPos.position.HasMoves()
}

// ValidateBestMoves validates the best moves for the position.
func (nPos *NormalizedPosition) ValidateBestMoves(bestMoves BestMoves) error {
	if bestMoves == nil {
		return errors.New("best moves is nil")
	}

	pos := nPos.Position()

	for _, move := range bestMoves {
		if !pos.IsValidMove(move) {
			return fmt.Errorf("invalid move: %d", move)
		}

		pos = pos.DoMove(move)
	}

	return nil
}

// ASCIIArtLines returns the ascii art lines for the position.
func (nPos *NormalizedPosition) ASCIIArtLines() []string {
	return nPos.position.ASCIIArtLines()
}

// ToProblem returns the problem string for the position.
func (nPos *NormalizedPosition) ToProblem() string {
	var squares string
	for i := range 64 {
		mask := uint64(1) << i
		switch {
		case mask&nPos.Player() != 0:
			squares += "X"
		case mask&nPos.Opponent() != 0:
			squares += "O"
		default:
			squares += "-"
		}
	}

	// Position does not store the turn, so we pretend to always be black
	return squares + " X;\n"
}
