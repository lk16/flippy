package othello

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
)

//go:generate go run ./gen precomputed_boards_12discs.txt

//go:embed precomputed_boards_12discs.txt
var precomputedBoards12Data string

var (
	precomputedBoards12     []NormalizedBoard
	precomputedBoards12Once sync.Once
)

// PrecomputedBoards12 returns the full set of NormalizedBoards with exactly
// 12 discs reachable by legal play from the standard starting position,
// excluding any where the player to move has no legal move (edax can't
// evaluate those, and their value is trivially the negation of whatever
// they pass into, so they'd never do anything but sit unlearned).
// Positions with fewer discs are never learned directly; they're backfilled
// by minimaxing this set instead.
func PrecomputedBoards12() []NormalizedBoard {
	precomputedBoards12Once.Do(func() {
		boards, err := parseNormalizedBoards(precomputedBoards12Data)
		if err != nil {
			panic("othello: corrupt embedded board data: " + err.Error())
		}
		precomputedBoards12 = boards
	})

	return append([]NormalizedBoard(nil), precomputedBoards12...)
}

// parseNormalizedBoards parses one Board.String() per line, in the format
// produced by the internal/othello/gen command, into NormalizedBoards.
func parseNormalizedBoards(data string) ([]NormalizedBoard, error) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	boards := make([]NormalizedBoard, len(lines))

	for i, line := range lines {
		board, err := ParseBoard(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		normalized, err := NewNormalizedBoard(board)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		boards[i] = normalized
	}

	return boards, nil
}
