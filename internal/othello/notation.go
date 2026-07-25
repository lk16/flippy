package othello

import (
	"fmt"
	"strings"
)

const fieldLength = 2

// parseField parses a 2-character move field such as "e6" into a 0-based
// board index. The case-insensitive fields "--", "ps", and "pa" parse to
// PassMove.
func parseField(field string) (int, error) {
	if len(field) != fieldLength {
		return 0, fmt.Errorf("invalid field length: %q", field)
	}

	field = strings.ToLower(field)

	switch field {
	case "--", "ps", "pa":
		return PassMove, nil
	}

	col, row := field[0], field[1]
	if col < 'a' || col > 'h' || row < '1' || row > '8' {
		return 0, fmt.Errorf("invalid field: %q", field)
	}

	x := int(col - 'a')
	y := int(row - '1')
	return y*boardWidth + x, nil
}
