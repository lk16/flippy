package othello

import (
	"fmt"
	"os"
	"strings"
)

// ParsePGNFile reads filename and parses it as a sequence of one or more PGN games.
func ParsePGNFile(filename string) ([]*Game, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ParsePGN(string(content), filename)
}

// ParsePGN parses content as PGN games; filename is a fallback time source when metadata has none.
func ParsePGN(content string, filename string) ([]*Game, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	var (
		games         []*Game
		metadataLines []string
		moveLines     []string
		inMetadata    = true
	)

	flush := func() error {
		game, err := newGameFromPGNLines(metadataLines, moveLines, filename)
		if err != nil {
			return err
		}

		games = append(games, game)
		metadataLines, moveLines = nil, nil
		return nil
	}

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "[") {
			if !inMetadata {
				if err := flush(); err != nil {
					return nil, err
				}
				inMetadata = true
			}
			metadataLines = append(metadataLines, line)
			continue
		}

		inMetadata = false
		moveLines = append(moveLines, line)
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return games, nil
}

func newGameFromPGNLines(metadataLines, moveLines []string, filename string) (*Game, error) {
	metadata, err := parsePGNMetadata(metadataLines, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	moves, err := parsePGNMoves(moveLines)
	if err != nil {
		return nil, fmt.Errorf("failed to parse moves: %w", err)
	}

	game, err := NewGameFromMoves(moves)
	if err != nil {
		return nil, fmt.Errorf("failed to create game: %w", err)
	}

	game.metadata = metadata
	game.filename = filename

	return game, nil
}

func parsePGNMoves(lines []string) ([]int, error) {
	var moves []int

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		for _, word := range strings.Fields(line) {
			// Skip move-number prefixes ("1.") and result tokens: the
			// digit-based ones ("1-0", "0-1", "1/2-1/2") and PGN's "*"
			// terminator for an unfinished or ongoing game.
			if word[0] >= '0' && word[0] <= '9' {
				continue
			}
			if word == "*" {
				continue
			}

			move, err := ParseField(word)
			if err != nil {
				return nil, fmt.Errorf("failed to parse move %q: %w", word, err)
			}

			moves = append(moves, move)
		}
	}

	return moves, nil
}
