package models

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"testing"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewNormalizedPositionFromString(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid position",
			input:      "00000000000000000000000000000000",
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "invalid length",
			input:      "0000000000000000000000000000000", // 31 chars
			wantErr:    true,
			wantErrMsg: "invalid player position",
		},
		{
			name:       "invalid hex",
			input:      "0000000000000000000000000000000G", // invalid hex char
			wantErr:    true,
			wantErrMsg: "invalid player position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nPos, err := NewNormalizedPositionFromString(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.input, nPos.String())
		})
	}
}

func TestNewNormalizedPositionFromBytes(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid bytes",
			input:      make([]byte, 16),
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "invalid length",
			input:      make([]byte, 15),
			wantErr:    true,
			wantErrMsg: "position bytes must be exactly 16 bytes, got 15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nPos, err := NewNormalizedPositionFromBytes(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.input, nPos.Bytes())
		})
	}
}

func TestNewNormalizedPositionEmpty(t *testing.T) {
	nPos := NewNormalizedPositionEmpty()
	assert.Equal(t, uint64(0x0), nPos.Player())
	assert.Equal(t, uint64(0x0), nPos.Opponent())
}

func generateTestNormalizedPositions(t *testing.T) []NormalizedPosition {
	positions := generateTestPositions(t)

	normalized := make([]NormalizedPosition, len(positions))
	for i, pos := range positions {
		normalizedPosition := pos.Normalized()
		normalized[i] = NewNormalizedPositionMust(normalizedPosition.Player(), normalizedPosition.Opponent())
	}
	return normalized
}

func TestNormalizedPosition_String(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		want := fmt.Sprintf("%016X%016X", nPos.Player(), nPos.Opponent())
		assert.Equal(t, want, nPos.String())
	}
}

func TestNormalizedPosition_Bytes(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		bytes := nPos.Bytes()
		assert.Len(t, bytes, 16)
		assert.Equal(t, nPos.Player(), binary.LittleEndian.Uint64(bytes[:8]))
		assert.Equal(t, nPos.Opponent(), binary.LittleEndian.Uint64(bytes[8:]))
	}
}

func TestNormalizedPosition_Scan(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		wantErr    bool
		wantErrMsg string
		want       NormalizedPosition
	}{
		{
			name:       "valid bytes",
			input:      make([]byte, 16),
			wantErr:    false,
			wantErrMsg: "",
			want:       NewNormalizedPositionEmpty(),
		},
		{
			name:       "not normalized",
			input:      []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			wantErr:    true,
			wantErrMsg: "position is not normalized",
			want:       NormalizedPosition{},
		},
		{
			name:       "nil value",
			input:      nil,
			wantErr:    true,
			wantErrMsg: "cannot scan nil into NormalizedPosition",
			want:       NormalizedPosition{},
		},
		{
			name:       "invalid type",
			input:      "not bytes",
			wantErr:    true,
			wantErrMsg: "cannot scan string into NormalizedPosition",
			want:       NormalizedPosition{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nPos NormalizedPosition
			err := nPos.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}

	for _, nPos := range generateTestNormalizedPositions(t) {
		var scannedNPos NormalizedPosition
		err := scannedNPos.Scan(nPos.Bytes())
		assert.NoError(t, err)
		assert.Equal(t, nPos, scannedNPos)
	}
}

func TestNormalizedPosition_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid json",
			input:      `"00000000000000000000000000000000"`,
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "invalid json",
			input:      `"invalid"`,
			wantErr:    true,
			wantErrMsg: "invalid player position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nPos NormalizedPosition
			err := nPos.UnmarshalJSON([]byte(tt.input))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}

	for _, nPos := range generateTestNormalizedPositions(t) {
		marshalled, err := json.Marshal(nPos.String())
		assert.NoError(t, err)

		var unmarshalled NormalizedPosition
		err = unmarshalled.UnmarshalJSON(marshalled)
		assert.NoError(t, err)
		assert.Equal(t, nPos, unmarshalled)
	}
}

func TestNormalizedPosition_MarshalJSON(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		bytes, err := nPos.MarshalJSON()
		assert.NoError(t, err)

		var unmarshalledString string
		err = json.Unmarshal(bytes, &unmarshalledString)
		assert.NoError(t, err)
		assert.Equal(t, nPos.String(), unmarshalledString)
	}
}

func TestNormalizedPosition_Normalized(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		for rotation := 0; rotation < 8; rotation++ {
			rotated := nPos.Position().rotate(rotation)

			if rotated.IsNormalized() {
				assert.Equal(t, nPos.Position(), rotated)
			} else {
				assert.NotEqual(t, nPos.Position(), rotated)
				assert.Equal(t, nPos.Position(), rotated.Normalized())
			}
		}
	}
}

func TestNormalizedPosition_Accessors(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {

		// Test shorthands
		assert.Equal(t, nPos.position.Player(), nPos.Player())
		assert.Equal(t, nPos.position.Opponent(), nPos.Opponent())
		assert.Equal(t, nPos.position, nPos.Position())
		assert.Equal(t, nPos.position.CountDiscs(), nPos.CountDiscs())
	}
}

// getNormalizedPositionWithMoves returns a normalized position with the given number of discs
// and ensures that the position has moves
func getNormalizedPositionWithMoves(discCount int) NormalizedPosition {
	var pos Position

	// Make sure position has moves
	for !pos.HasMoves() {
		var err error
		pos, err = NewPositionRandom(discCount)
		if err != nil {
			log.Fatalf("error creating position: %s", err)
		}
	}

	pos = pos.Normalized()
	return NewNormalizedPositionMust(pos.Player(), pos.Opponent())
}

func TestNormalizedPosition_IsDbSavable(t *testing.T) {
	tests := []struct {
		name string
		nPos NormalizedPosition
		want bool
	}{
		{
			name: "empty",
			nPos: NewNormalizedPositionEmpty(),
			want: false,
		},
		{
			name: "too many discs",
			nPos: getNormalizedPositionWithMoves(config.MaxBookSavableDiscs + 1),
			want: false,
		},
		{
			name: "no moves",
			nPos: NewNormalizedPositionMust(0xFF, 0x0),
			want: false,
		},
		{
			name: "valid max discs",
			nPos: getNormalizedPositionWithMoves(config.MaxBookSavableDiscs),
			want: true,
		},
		{
			name: "valid min discs",
			nPos: getNormalizedPositionWithMoves(4),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.nPos.IsDbSavable())
		})
	}
}

func TestNormalizedPosition_HasMoves(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		assert.Equal(t, nPos.Position().HasMoves(), nPos.HasMoves())
	}
}

func TestNormalizedPosition_ValidateBestMoves(t *testing.T) {
	nPos := getNormalizedPositionWithMoves(4)

	tests := []struct {
		name       string
		bestMoves  BestMoves
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid",
			bestMoves:  BestMoves{19, 18},
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "invalid last move",
			bestMoves:  BestMoves{19, 18, 63},
			wantErr:    true,
			wantErrMsg: "invalid move: 63",
		},
		{
			name:       "invalid first move",
			bestMoves:  BestMoves{63, 18, 17},
			wantErr:    true,
			wantErrMsg: "invalid move: 63",
		},
		{
			name:       "invalid middle move",
			bestMoves:  BestMoves{19, 63, 17},
			wantErr:    true,
			wantErrMsg: "invalid move: 63",
		},
		{
			name:       "move out of bounds",
			bestMoves:  BestMoves{19, -8, 17},
			wantErr:    true,
			wantErrMsg: "invalid move: -8",
		},
		{
			name:       "no moves",
			bestMoves:  BestMoves{},
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "nil",
			bestMoves:  nil,
			wantErr:    true,
			wantErrMsg: "best moves is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := nPos.ValidateBestMoves(tt.bestMoves)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
