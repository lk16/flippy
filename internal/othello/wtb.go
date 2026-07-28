package othello

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	wtbHeaderSize     = 16
	wtbGameRecordSize = 68
	wtbMoveBytesStart = 8
)

// ParseWTBFile reads filename and parses it as a WTHOR (.wtb) archive.
func ParseWTBFile(filename string) ([]*Game, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ParseWTB(data)
}

// ParseWTB parses data as a WTHOR archive: a 16-byte header (game count at offset 4) plus one 68-byte record per game.
func ParseWTB(data []byte) ([]*Game, error) {
	if len(data) < wtbHeaderSize {
		return nil, fmt.Errorf("wtb data too short for header: %d bytes", len(data))
	}

	gameCount := binary.LittleEndian.Uint32(data[4:8])
	records := data[wtbHeaderSize:]

	games := make([]*Game, 0, gameCount)

	for i := range gameCount {
		start := int(i) * wtbGameRecordSize
		end := start + wtbGameRecordSize

		if end > len(records) {
			return nil, fmt.Errorf("wtb data too short for game %d", i)
		}

		game, err := newGameFromWTBRecord(records[start:end])
		if err != nil {
			return nil, fmt.Errorf("failed to parse game %d: %w", i, err)
		}

		games = append(games, game)
	}

	return games, nil
}

// newGameFromWTBRecord builds a Game from a 68-byte WTHOR record: 8 skipped bytes, then one
// row*10+col (1-based) move byte per ply, terminated by a zero byte.
func newGameFromWTBRecord(record []byte) (*Game, error) {
	moveBytes := record[wtbMoveBytesStart:wtbGameRecordSize]

	moves := make([]int, 0, len(moveBytes))

	for _, moveByte := range moveBytes {
		if moveByte == 0 {
			break
		}

		row, col := int(moveByte)/10, int(moveByte)%10
		moves = append(moves, (row-1)*boardWidth+(col-1))
	}

	return NewGameFromMoves(moves)
}
