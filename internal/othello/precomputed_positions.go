package othello

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
)

//go:generate go run ./gen precomputed_positions_12discs.txt

//go:embed precomputed_positions_12discs.txt
var precomputedPositions12Data string

var (
	precomputedPositions12     []NormalizedPosition
	precomputedPositions12Once sync.Once
)

// PrecomputedPositions12 returns every reachable 12-disc NormalizedPosition, excluding
// no-legal-move positions since edax can't evaluate them.
func PrecomputedPositions12() []NormalizedPosition {
	precomputedPositions12Once.Do(func() {
		positions, err := parseNormalizedPositions(precomputedPositions12Data)
		if err != nil {
			panic("othello: corrupt embedded position data: " + err.Error())
		}
		precomputedPositions12 = positions
	})

	return append([]NormalizedPosition(nil), precomputedPositions12...)
}

// parseNormalizedPositions parses one Position.String() per line into NormalizedPositions.
func parseNormalizedPositions(data string) ([]NormalizedPosition, error) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	positions := make([]NormalizedPosition, len(lines))

	for i, line := range lines {
		position, err := ParsePosition(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		normalized, err := NewNormalizedPosition(position)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		positions[i] = normalized
	}

	return positions, nil
}
