package models

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
)

// NormalizedPosition represents a normalized position on the board
// We make sure it's normalized by normalizing it when we create it.
type NormalizedPosition struct {
	position Position
}

// NewNormalizedPositionFromString creates a new normalized position from a string
func NewNormalizedPositionFromString(s string) (NormalizedPosition, error) {
	if len(s) != 32 {
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

	return newNormalizedPosition(player, opponent)
}

// NewNormalizedPositionFromBytes creates a new normalized position from a byte slice
func NewNormalizedPositionFromBytes(b []byte) (NormalizedPosition, error) {
	if len(b) != 16 {
		return NormalizedPosition{}, fmt.Errorf("position bytes must be exactly 16 bytes, got %d", len(b))
	}

	player := binary.LittleEndian.Uint64(b[:8])
	opponent := binary.LittleEndian.Uint64(b[8:])

	return newNormalizedPosition(player, opponent)
}

// NewNormalizedPositionEmpty creates a new normalized position with no discs
func NewNormalizedPositionEmpty() NormalizedPosition {
	normalized, err := NewPosition(0, 0)
	if err != nil {
		panic(fmt.Sprintf("normalized empty position is invalid: %s", err))
	}

	return NormalizedPosition{
		position: normalized,
	}
}

// newNormalizedPosition creates a new normalized position from a player and opponent bitboard
func newNormalizedPosition(player, opponent uint64) (NormalizedPosition, error) {
	pos, err := NewPosition(player, opponent)
	if err != nil {
		return NormalizedPosition{}, fmt.Errorf("invalid normalized position: %w", err)
	}

	if !pos.IsNormalized() {
		return NormalizedPosition{}, fmt.Errorf("invalid normalized position: position is not normalized")
	}

	return NormalizedPosition{
		position: pos,
	}, nil
}

// String implements the Stringer interface for Position.
// It returns a 32-character hex string where the first 16 characters represent the player's pieces
// and the last 16 characters represent the opponent's pieces.
func (n NormalizedPosition) String() string {
	return fmt.Sprintf("%016X%016X", n.Player(), n.Opponent())
}

// Bytes returns the normalized position as a byte slice
func (n NormalizedPosition) Bytes() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[:8], n.Player())
	binary.LittleEndian.PutUint64(b[8:], n.Opponent())
	return b
}

// UnmarshalJSON implements json.Unmarshaler for Position.
// It expects a 32-character hex string where the first 16 characters represent the player's pieces
// and the last 16 characters represent the opponent's pieces.
func (n *NormalizedPosition) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid position string: %w", err)
	}

	var err error
	*n, err = NewNormalizedPositionFromString(s)
	if err != nil {
		return fmt.Errorf("invalid position string: %w", err)
	}

	return nil
}

// MarshalJSON implements json.Marshaler for Position.
// It returns a 32-character hex string where the first 16 characters represent the player's pieces
// and the last 16 characters represent the opponent's pieces.
func (n NormalizedPosition) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.String())
}

// Player returns the player bitboard
func (n NormalizedPosition) Player() uint64 {
	return n.position.Player()
}

// Opponent returns the opponent bitboard
func (n NormalizedPosition) Opponent() uint64 {
	return n.position.Opponent()
}

// CountDiscs returns the number of discs on the board
func (n NormalizedPosition) CountDiscs() int {
	return n.position.CountDiscs()
}

// Scan implements the sql.Scanner interface for NormalizedPosition
func (n *NormalizedPosition) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("cannot scan nil into NormalizedPosition")
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into NormalizedPosition", value)
	}

	pos, err := NewNormalizedPositionFromBytes(bytes)
	if err != nil {
		return fmt.Errorf("error scanning position: %w", err)
	}

	*n = pos
	return nil
}
