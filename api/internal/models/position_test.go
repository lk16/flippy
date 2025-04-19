package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlipHorizontally(t *testing.T) {
	for i := 0; i < 64; i++ {
		x := uint64(1 << i)

		row := i / 8
		col := i % 8

		flippedIndex := row*8 + 7 - col

		flipped := flipHorizontally(x)
		wantFlipped := uint64(1 << flippedIndex)
		assert.Equal(t, flipped, wantFlipped)
	}
}

func TestFlipVertically(t *testing.T) {
	for i := 0; i < 64; i++ {
		x := uint64(1 << i)

		row := i / 8
		col := i % 8

		flippedIndex := 8*(7-row) + col

		flipped := flipVertically(x)
		wantFlipped := uint64(1 << flippedIndex)
		assert.Equal(t, flipped, wantFlipped)
	}
}

func TestFlipDiagonally(t *testing.T) {
	for i := 0; i < 64; i++ {
		x := uint64(1 << i)

		row := i / 8
		col := i % 8

		flippedIndex := 8*col + row

		flipped := flipDiagonally(x)
		wantFlipped := uint64(1 << flippedIndex)
		assert.Equal(t, flipped, wantFlipped)
	}
}

func TestNewPosition(t *testing.T) {
	tests := []struct {
		name       string
		player     uint64
		opponent   uint64
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid position",
			player:     0x0000000000000000,
			opponent:   0x0000000000000000,
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "invalid position",
			player:     0x0000000000000001,
			opponent:   0x0000000000000001,
			wantErr:    true,
			wantErrMsg: "invalid position: player and opponent discs cannot overlap",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewPosition(test.player, test.opponent)
			if test.wantErr {
				assert.Error(t, err)
				assert.Equal(t, test.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPosition_Rotate(t *testing.T) {
	pos := Position{
		player:   0x0000000000000001,
		opponent: 0x8000000000000000,
	}

	for i := 0; i < 8; i++ {
		assert.True(t, pos.rotate(i).unrotate(i).equals(pos))
	}
}

func TestPosition_Normalize(t *testing.T) {
	pos := Position{
		player:   0x8000000000000000,
		opponent: 0x0000000000000000,
	}

	normalized, rotation := pos.Normalize()
	assert.NotEqual(t, 0, rotation)

	assert.True(t, pos.rotate(rotation).equals(normalized))
}

func TestPosition_Normalized(t *testing.T) {
	pos := Position{
		player:   0x8000000000000000,
		opponent: 0x0000000000000000,
	}

	normalized := pos.Normalized()
	wantNormalized, _ := pos.Normalize()
	assert.Equal(t, wantNormalized, normalized)
}

func TestPosition_IsNormalized(t *testing.T) {
	pos := Position{
		player:   0x8000000000000000,
		opponent: 0x0000000000000000,
	}

	assert.False(t, pos.IsNormalized())
	assert.True(t, pos.Normalized().IsNormalized())
}

func TestPosition_Player(t *testing.T) {
	pos := Position{
		player:   0x8000000000000000,
		opponent: 0x0000000000000001,
	}

	assert.Equal(t, uint64(0x8000000000000000), pos.Player())
}

func TestPosition_Opponent(t *testing.T) {
	pos := Position{
		player:   0x8000000000000000,
		opponent: 0x0000000000000001,
	}

	assert.Equal(t, uint64(0x0000000000000001), pos.Opponent())
}

func TestPosition_CountDiscs(t *testing.T) {
	tests := []struct {
		name     string
		position Position
		want     int
	}{
		{
			name: "empty board",
			position: Position{
				player:   0x0000000000000000,
				opponent: 0x0000000000000000,
			},
			want: 0,
		},
		{
			name: "single disc",
			position: Position{
				player:   0x8000000000000000,
				opponent: 0x0000000000000000,
			},
			want: 1,
		},
		{
			name: "multiple discs",
			position: Position{
				player:   0x8100000000000000,
				opponent: 0x0000000000000001,
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.position.CountDiscs()
			assert.Equal(t, tt.want, got)
		})
	}
}
