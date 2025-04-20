package models

import (
	"math/bits"
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

func TestNewPositionMust(t *testing.T) {
	tests := []struct {
		name      string
		player    uint64
		opponent  uint64
		wantPanic bool
	}{
		{
			name:      "valid position",
			player:    0x0000000000000000,
			opponent:  0x0000000000000000,
			wantPanic: false,
		},
		{
			name:      "invalid position",
			player:    0x0000000000000001,
			opponent:  0x0000000000000001,
			wantPanic: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.wantPanic {
				assert.Panics(t, func() { NewPositionMust(test.player, test.opponent) })
			} else {
				pos := NewPositionMust(test.player, test.opponent)
				assert.Equal(t, test.player, pos.Player())
				assert.Equal(t, test.opponent, pos.Opponent())
			}
		})
	}
}

func TestNewPositionRandom(t *testing.T) {
	tests := []struct {
		name       string
		discs      int
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid position",
			discs:      4,
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "valid position",
			discs:      64,
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "invalid number of discs",
			discs:      3,
			wantErr:    true,
			wantErrMsg: "invalid number of discs: 3",
		},
		{
			name:       "invalid number of discs",
			discs:      65,
			wantErr:    true,
			wantErrMsg: "invalid number of discs: 65",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pos, err := NewPositionRandom(test.discs)
			if test.wantErr {
				assert.Error(t, err)
				assert.Equal(t, test.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.discs, pos.CountDiscs())
			}
		})
	}

	for i := 4; i <= 64; i++ {
		pos, err := NewPositionRandom(i)
		assert.NoError(t, err)
		assert.Equal(t, i, pos.CountDiscs())
	}
}

func TestNewPositionStart(t *testing.T) {
	pos := NewPositionStart()
	assert.Equal(t, 4, pos.CountDiscs())
	assert.Equal(t, uint64(0x0000000810000000), pos.Player())
	assert.Equal(t, uint64(0x0000001008000000), pos.Opponent())
}

func TestNewPositionEmpty(t *testing.T) {
	pos := NewPositionEmpty()
	assert.Equal(t, 0, pos.CountDiscs())
	assert.Equal(t, uint64(0x0000000000000000), pos.Player())
	assert.Equal(t, uint64(0x0000000000000000), pos.Opponent())
}

func TestPosition_equals(t *testing.T) {
	pos1 := NewPositionMust(0x0000000000000000, 0x0000000000000000)
	pos2 := NewPositionMust(0x0000000000000000, 0x0000000000000001)
	assert.Equal(t, pos1, pos1)
	assert.NotEqual(t, pos1, pos2)
}

// generateTestPositions generates some positions used for testing
func generateTestPositions(t *testing.T) []Position {
	positions := make([]Position, 0)

	// Generate all boards with all flipping lines from each square
	for y := uint(0); y < 8; y++ {
		for x := uint(0); x < 8; x++ {
			player := uint64(1 << (y*8 + x))

			// for each direction
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if (dy == 0) && (dx == 0) {
						continue
					}
					opponent := uint64(0)

					// for each distance
					for d := 1; d <= 6; d++ {

						// check if me can still flip within othello boundaries
						py := int(y) + (d+1)*dy
						px := int(x) + (d+1)*dx

						if (py < 0) || (py > 7) || (px < 0) || (px > 7) {
							break
						}

						qy := y + uint(d*dy)
						qx := x + uint(d*dx)

						opponent |= uint64(1 << (qy*8 + qx))

						position := NewPositionMust(player, opponent)
						positions = append(positions, position)
					}
				}
			}
		}
	}

	// Add some common Boards
	positions = append(positions, NewPositionEmpty())
	positions = append(positions, NewPositionStart())

	// Add some full boards
	positions = append(positions, NewPositionMust(0xFFFFFFFFFFFFFFFF, 0x0000000000000000))
	positions = append(positions, NewPositionMust(0x0000000000000000, 0xFFFFFFFFFFFFFFFF))
	positions = append(positions, NewPositionMust(0xFFFFFFFF00000000, 0x00000000FFFFFFFF))
	positions = append(positions, NewPositionMust(0x00000000FFFFFFFF, 0xFFFFFFFF00000000))
	positions = append(positions, NewPositionMust(0x5555555555555555, 0xAAAAAAAAAAAAAAAA))
	positions = append(positions, NewPositionMust(0xAAAAAAAAAAAAAAAA, 0x5555555555555555))

	// Add random reachable boards with 4-64 discs
	for i := 0; i < 10; i++ {
		for discs := 4; discs <= 64; discs++ {
			position, err := NewPositionRandom(discs)
			assert.NoError(t, err)
			positions = append(positions, position)
		}
	}

	return positions
}

func TestPosition_Rotate(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		for i := 0; i < 8; i++ {
			assert.Equal(t, pos, pos.rotate(i).unrotate(i))
		}
	}
}

func TestPosition_Normalize(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		normalized, rotation := pos.Normalize()
		assert.Equal(t, normalized, pos.rotate(rotation))
		assert.Equal(t, pos, normalized.unrotate(rotation))
	}
}

func TestPosition_Normalized(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		normalized := pos.Normalized()
		wantNormalized, _ := pos.Normalize()
		assert.Equal(t, wantNormalized, normalized)
	}
}

func TestPosition_IsNormalized(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		assert.Equal(t, pos.IsNormalized(), pos.Normalized() == pos)
	}
}

func TestPosition_Player(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		assert.Equal(t, pos.Player(), pos.player)
	}
}

func TestPosition_Opponent(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		assert.Equal(t, pos.Opponent(), pos.opponent)
	}
}

func TestPosition_CountDiscs(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		want := bits.OnesCount64(pos.player | pos.opponent)
		assert.Equal(t, want, pos.CountDiscs())
	}
}

func TestPosition_HasMoves(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		assert.Equal(t, pos.HasMoves(), pos.Moves() != 0)
	}
}

func TestPosition_IsValidMove(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		for i := 0; i < 64; i++ {
			want := pos.Moves()&(1<<i) != 0
			assert.Equal(t, want, pos.IsValidMove(i))
		}
	}
}

func TestPosition_Moves(t *testing.T) {
	for _, pos := range generateTestPositions(t) {

		wantMoves := uint64(0)
		for i := 0; i < 64; i++ {
			if pos.flipped(i) != 0 {
				wantMoves |= (1 << i)
			}
		}

		assert.Equal(t, pos.Moves(), wantMoves)
	}
}

func TestPosition_DoMove(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		validMoves := pos.Moves()
		for i := 0; i < 64; i++ {
			if validMoves&(1<<i) == 0 {
				assert.Equal(t, pos, pos.DoMove(i))
			} else {
				after := pos.DoMove(i)
				flipped := pos.flipped(i)
				wantPlayer := pos.opponent &^ flipped
				wantOpponent := pos.player | flipped | (1 << i)
				assert.Equal(t, wantPlayer, after.player)
				assert.Equal(t, wantOpponent, after.opponent)
			}
		}
	}
}

// flippedSlow is a slow implementation of the flipped function to verify the correctness of the fast implementation
func flippedSlow(pos Position, move int) uint64 {
	if (pos.player|pos.opponent)&(1<<move) != 0 {
		return 0
	}

	flipped := uint64(0)
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			s := 1
			for {
				curx := int(move%8) + (dx * s)
				cury := int(move/8) + (dy * s)
				if curx < 0 || curx >= 8 || cury < 0 || cury >= 8 {
					break
				}

				cur := uint(8*cury + curx)

				if pos.opponent&(1<<cur) != 0 {
					s++
				} else {
					if pos.player&(1<<cur) != 0 && (s >= 2) {
						for p := 1; p < s; p++ {
							f := move + (p * (8*dy + dx))
							flipped |= (1 << f)
						}
					}
					break
				}
			}
		}
	}

	return flipped
}

func TestPosition_flipped(t *testing.T) {
	for _, pos := range generateTestPositions(t) {
		for i := 0; i < 64; i++ {
			assert.Equal(t, pos.flipped(i), flippedSlow(pos, i))
		}
	}
}
