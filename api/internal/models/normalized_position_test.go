package models

import (
	"encoding/binary"
	"testing"

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
			pos, err := NewNormalizedPositionFromString(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.input, pos.String())
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
			pos, err := NewNormalizedPositionFromBytes(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.input, pos.Bytes())
		})
	}
}

func TestNormalizedPosition_String(t *testing.T) {
	pos := NormalizedPosition{
		position: Position{
			player:   0x0000000000000001,
			opponent: 0x8000000000000000,
		},
	}
	assert.Equal(t, "00000000000000018000000000000000", pos.String())
}

func TestNormalizedPosition_Bytes(t *testing.T) {
	pos := NormalizedPosition{
		position: Position{
			player:   0x0000000000000001,
			opponent: 0x8000000000000000,
		},
	}
	bytes := pos.Bytes()
	assert.Len(t, bytes, 16)
	assert.Equal(t, uint64(0x0000000000000001), binary.LittleEndian.Uint64(bytes[:8]))
	assert.Equal(t, uint64(0x8000000000000000), binary.LittleEndian.Uint64(bytes[8:]))
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
			want: NormalizedPosition{
				position: Position{
					player:   0x0000000000000000,
					opponent: 0x0000000000000000,
				},
			},
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
			var pos NormalizedPosition
			err := pos.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
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
			var pos NormalizedPosition
			err := pos.UnmarshalJSON([]byte(tt.input))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestNormalizedPosition_MarshalJSON(t *testing.T) {
	pos := NormalizedPosition{
		position: Position{
			player:   0x0000000000000001,
			opponent: 0x8000000000000000,
		},
	}
	bytes, err := pos.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, `"00000000000000018000000000000000"`, string(bytes))
}

func TestNormalizedPosition_Normalized(t *testing.T) {
	normalized, err := newNormalizedPosition(0x8000000, 0x3810000000)
	assert.NoError(t, err)

	for rotation := 0; rotation < 8; rotation++ {
		rotated := normalized.Position().rotate(rotation)

		if rotated.IsNormalized() {
			assert.Equal(t, normalized.Position(), rotated)
			assert.Equal(t, 0, rotation)
		} else {
			assert.True(t, normalized.Position().Equals(rotated.Normalized()))
			assert.NotEqual(t, 0, rotation)
		}

	}
}

func TestNormalizedPosition_Accessors(t *testing.T) {
	tests := []struct {
		name           string
		position       Position
		wantPlayer     uint64
		wantOpponent   uint64
		wantCountDiscs int
	}{
		{
			name: "empty board",
			position: Position{
				player:   0x0000000000000000,
				opponent: 0x0000000000000000,
			},
			wantPlayer:     0x0000000000000000,
			wantOpponent:   0x0000000000000000,
			wantCountDiscs: 0,
		},
		{
			name: "single disc each",
			position: Position{
				player:   0x0000000000000001,
				opponent: 0x8000000000000000,
			},
			wantPlayer:     0x0000000000000001,
			wantOpponent:   0x8000000000000000,
			wantCountDiscs: 2,
		},
		{
			name: "multiple discs",
			position: Position{
				player:   0x0000000000000003, // 2 discs
				opponent: 0xC000000000000000, // 2 discs
			},
			wantPlayer:     0x0000000000000003,
			wantOpponent:   0xC000000000000000,
			wantCountDiscs: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := NormalizedPosition{
				position: tt.position,
			}

			// Test Player()
			assert.Equal(t, tt.wantPlayer, pos.Player())

			// Test Opponent()
			assert.Equal(t, tt.wantOpponent, pos.Opponent())

			// Test Position()
			assert.Equal(t, tt.position, pos.Position())

			// Test CountDiscs()
			assert.Equal(t, tt.wantCountDiscs, pos.CountDiscs())
		})
	}
}
