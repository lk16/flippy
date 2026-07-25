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

// ParseWTB parses data as a WTHOR (.wtb) archive: a 16-byte header (whose
// only field we use is a 4-byte game count at offset 4) followed by one
// 68-byte record per game.
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

// newGameFromWTBRecord builds a Game from a 68-byte WTHOR game record. The
// first 8 bytes (tournament/player ids, recorded scores) aren't needed to
// reconstruct the game and are skipped. The remaining 60 bytes hold one move
// per ply, encoded as row*10+col with row and col both 1-based; a zero byte
// marks the end of the moves.
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
