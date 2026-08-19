package othello

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// encodeWTBRecord builds a single 68-byte WTHOR game record, encoding each move as row*10+col
// with row/col 1-based, matching the format ParseWTB decodes.
func encodeWTBRecord(moves []int) []byte {
	record := make([]byte, wtbGameRecordSize)

	for i, move := range moves {
		x, y := move%boardWidth, move/boardWidth
		record[wtbMoveBytesStart+i] = byte((y+1)*10 + (x + 1))
	}

	return record
}

func encodeWTB(gamesMoves [][]int) []byte {
	data := make([]byte, wtbHeaderSize)
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(gamesMoves)))

	for _, moves := range gamesMoves {
		data = append(data, encodeWTBRecord(moves)...)
	}

	return data
}

func TestParseWTB_RoundTrip(t *testing.T) {
	gamesMoves := [][]int{
		{19, 18, 17, 9, 1, 0, 37, 43, 51, 2},
		{19, 18, 17, 9},
		{},
	}

	games, err := ParseWTB(encodeWTB(gamesMoves))
	require.NoError(t, err)
	require.Len(t, games, len(gamesMoves))

	for i, moves := range gamesMoves {
		// Build the expected game the same way ParseWTB does, so this only tests the byte
		// round-trip and not the separately tested auto-pass logic.
		want, err := NewGameFromMoves(moves)
		require.NoError(t, err)
		require.Equal(t, want.Moves(), games[i].Moves())
	}
}

func TestParseWTB_TooShortHeader(t *testing.T) {
	_, err := ParseWTB(make([]byte, wtbHeaderSize-1))
	require.Error(t, err)
}

func TestParseWTB_TruncatedRecord(t *testing.T) {
	data := encodeWTB([][]int{{19, 18}})
	_, err := ParseWTB(data[:len(data)-1])
	require.Error(t, err)
}

func TestParseWTB_IllegalMove(t *testing.T) {
	// Index 0 (a1) is not a legal opening move.
	_, err := ParseWTB(encodeWTB([][]int{{0}}))
	require.Error(t, err)
}

func TestParseWTBFile_NotFound(t *testing.T) {
	_, err := ParseWTBFile("testdata/wtb/does-not-exist.wtb")
	require.Error(t, err)
}

func TestParseWTBFile_RealArchive(t *testing.T) {
	// Smoke test against a small real WTHOR archive.
	games, err := ParseWTBFile("testdata/wtb/WTH_1977.wtb")
	require.NoError(t, err)
	require.Len(t, games, 12)

	for _, game := range games {
		require.NotEmpty(t, game.Moves())
	}
}

// TestParseWTB_HugeGameCountDoesNotOverAllocate covers a hostile header claiming more games than
// the data holds: parsing must fail cleanly rather than pre-allocate from the header's count.
func TestParseWTB_HugeGameCountDoesNotOverAllocate(t *testing.T) {
	data := make([]byte, wtbHeaderSize)
	binary.LittleEndian.PutUint32(data[4:8], math.MaxUint32)

	_, err := ParseWTB(data)
	require.Error(t, err)
}
