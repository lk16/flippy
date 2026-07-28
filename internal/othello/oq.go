package othello

import "fmt"

// ParseOthelloQuestMoves parses a compact move string, e.g. "e6f4e3d6", into a Game (see ParseField).
func ParseOthelloQuestMoves(s string) (*Game, error) {
	if len(s)%fieldLength != 0 {
		return nil, fmt.Errorf("move string length %d is not a multiple of %d", len(s), fieldLength)
	}

	moves := make([]int, 0, len(s)/fieldLength)

	for i := 0; i < len(s); i += fieldLength {
		field := s[i : i+fieldLength]

		move, err := ParseField(field)
		if err != nil {
			return nil, fmt.Errorf("failed to parse move %q: %w", field, err)
		}

		moves = append(moves, move)
	}

	return NewGameFromMoves(moves)
}
