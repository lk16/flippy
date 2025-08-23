package models

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lk16/flippy/api/internal/config"
	"github.com/stretchr/testify/require"
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
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.input, nPos.String())
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
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.input, nPos.Bytes())
		})
	}
}

func TestNewNormalizedPositionEmpty(t *testing.T) {
	nPos := NewNormalizedPositionEmpty()
	require.Equal(t, uint64(0x0), nPos.Player())
	require.Equal(t, uint64(0x0), nPos.Opponent())
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
		require.Equal(t, want, nPos.String())
	}
}

func TestNormalizedPosition_Bytes(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		bytes := nPos.Bytes()
		require.Len(t, bytes, 16)
		require.Equal(t, nPos.Player(), binary.LittleEndian.Uint64(bytes[:8]))
		require.Equal(t, nPos.Opponent(), binary.LittleEndian.Uint64(bytes[8:]))
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
			name: "not normalized",
			input: []byte{
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x00,
				0x01,
			},
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
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}

	for _, nPos := range generateTestNormalizedPositions(t) {
		var scannedNPos NormalizedPosition
		err := scannedNPos.Scan(nPos.Bytes())
		require.NoError(t, err)
		require.Equal(t, nPos, scannedNPos)
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
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}

	for _, nPos := range generateTestNormalizedPositions(t) {
		marshalled, err := json.Marshal(nPos.String())
		require.NoError(t, err)

		var unmarshalled NormalizedPosition
		err = unmarshalled.UnmarshalJSON(marshalled)
		require.NoError(t, err)
		require.Equal(t, nPos, unmarshalled)
	}
}

func TestNormalizedPosition_MarshalJSON(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		bytes, err := nPos.MarshalJSON()
		require.NoError(t, err)

		var unmarshalledString string
		err = json.Unmarshal(bytes, &unmarshalledString)
		require.NoError(t, err)
		require.Equal(t, nPos.String(), unmarshalledString)
	}
}

func TestNormalizedPosition_Normalized(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		for rotation := range 8 {
			rotated := nPos.Position().rotate(rotation)

			if rotated.IsNormalized() {
				require.Equal(t, nPos.Position(), rotated)
			} else {
				require.NotEqual(t, nPos.Position(), rotated)
				require.Equal(t, nPos, rotated.Normalized())
			}
		}
	}
}

func TestNormalizedPosition_Accessors(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		// Test shorthands
		require.Equal(t, nPos.position.Player(), nPos.Player())
		require.Equal(t, nPos.position.Opponent(), nPos.Opponent())
		require.Equal(t, nPos.position, nPos.Position())
		require.Equal(t, nPos.position.CountDiscs(), nPos.CountDiscs())
	}
}

// and ensures that the position has moves.
func getNormalizedPositionWithMoves(discCount int) (*NormalizedPosition, error) {
	var pos Position

	// Make sure position has moves
	for !pos.HasMoves() {
		var err error
		pos, err = NewPositionRandom(discCount)
		if err != nil {
			return nil, err
		}
	}

	normalized := pos.Normalized()

	return &normalized, nil
}

func TestNormalizedPosition_IsDbSavable(t *testing.T) {
	tooManyDiscs, err := getNormalizedPositionWithMoves(config.MaxBookSavableDiscs + 1)
	require.NoError(t, err)

	noMoves, err := getNormalizedPositionWithMoves(4)
	require.NoError(t, err)

	valid, err := getNormalizedPositionWithMoves(config.MaxBookSavableDiscs)
	require.NoError(t, err)

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
			nPos: *tooManyDiscs,
			want: false,
		},
		{
			name: "no moves",
			nPos: NewNormalizedPositionMust(0xFF, 0x0),
			want: false,
		},
		{
			name: "valid max discs",
			nPos: *valid,
			want: true,
		},
		{
			name: "valid min discs",
			nPos: *noMoves,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.nPos.IsDBSavable())
		})
	}
}

func TestNormalizedPosition_HasMoves(t *testing.T) {
	for _, nPos := range generateTestNormalizedPositions(t) {
		require.Equal(t, nPos.Position().HasMoves(), nPos.HasMoves())
	}
}

func TestNormalizedPosition_ValidateBestMoves(t *testing.T) {
	nPos, err := getNormalizedPositionWithMoves(4)
	require.NoError(t, err)

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
			err = nPos.ValidateBestMoves(tt.bestMoves)
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
