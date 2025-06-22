package models

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TODO add tests for Game

// Game represents a complete Othello game.
type Game struct {
	File     string
	Metadata map[string]string
	Boards   []Board
	Moves    []int
}

// NewGame creates a new empty game.
func NewGame() *Game {
	return &Game{
		Metadata: make(map[string]string),
		Boards:   make([]Board, 0),
		Moves:    make([]int, 0),
	}
}

// NewGameFromPGN creates a new game from a PGN file.
func NewGameFromPGN(file string) (*Game, error) {
	contents, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	game, err := NewGameFromString(string(contents))
	if err != nil {
		return nil, fmt.Errorf("failed to parse PGN: %w", err)
	}

	game.File = file
	return game, nil
}

// NewGameFromMoves creates a new game from a list of moves.
func NewGameFromMoves(moves []int) (*Game, error) {
	board := NewBoardStart()
	boards := []Board{board}

	for _, move := range moves {
		if !board.IsValidMove(move) {
			// Passed moves may be missing from moves list
			board = board.DoMove(PassMove)
			boards = append(boards, board)
		}

		board = board.DoMove(move)
		boards = append(boards, board)
	}

	game := NewGame()
	game.Boards = boards
	game.Moves = append([]int{}, moves...)
	return game, nil
}

// NewGameFromString creates a new game from a PGN string.
func NewGameFromString(content string) (*Game, error) { //nolint:gocognit
	game := NewGame()

	lines := strings.Split(content, "\n")
	lineOffset := 0

	// Parse metadata
	metadataRegex := regexp.MustCompile(`\[(.*) "(.*)"\]`)
	for i, line := range lines {
		if !strings.HasPrefix(line, "[") {
			lineOffset = i
			break
		}

		matches := metadataRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			return nil, fmt.Errorf("could not parse PGN metadata: %s", line)
		}

		key := matches[1]
		value := matches[2]
		game.Metadata[key] = value
	}

	// Parse moves
	board := NewBoardStart()
	game.Boards = append(game.Boards, board)

	for _, line := range lines[lineOffset:] {
		if strings.TrimSpace(line) == "" {
			continue
		}

		words := strings.Fields(line)
		for _, word := range words {
			if len(word) == 0 || word[0] >= '0' && word[0] <= '9' {
				continue
			}

			move, err := FieldToIndex(word)
			if err != nil {
				return nil, fmt.Errorf("failed to parse move %s: %w", word, err)
			}

			if !board.IsValidMove(move) {
				// Some PGN's don't mark passed moves properly
				board = board.DoMove(PassMove)
				game.Boards = append(game.Boards, board)
			}

			board = board.DoMove(move)
			game.Moves = append(game.Moves, move)
			game.Boards = append(game.Boards, board)
		}
	}

	return game, nil
}

// IsXOT checks if this is an XOT variant game.
func (g *Game) IsXOT() bool {
	variant, exists := g.Metadata["Variant"]
	return exists && variant == "xot"
}

// GetDate returns the game date.
func (g *Game) GetDate() (time.Time, error) {
	dateStr, exists := g.Metadata["Date"]
	if !exists {
		return time.Time{}, errors.New("no date in metadata")
	}

	return time.Parse("2006.01.02", dateStr)
}

// GetDateTime returns the game date and time.
func (g *Game) GetDateTime() (time.Time, error) {
	dateStr, exists := g.Metadata["Date"]
	if !exists {
		return time.Time{}, errors.New("no date in metadata")
	}

	timeStr, exists := g.Metadata["Time"]
	if !exists {
		// Return just the date
		return time.Parse("2006.01.02", dateStr)
	}

	return time.Parse("2006.01.02 15:04:05", dateStr+" "+timeStr)
}

// GetWhitePlayer returns the white player name.
func (g *Game) GetWhitePlayer() string {
	return g.Metadata["White"]
}

// GetBlackPlayer returns the black player name.
func (g *Game) GetBlackPlayer() string {
	return g.Metadata["Black"]
}

// GetWinningPlayer returns the winning player name.
func (g *Game) GetWinningPlayer() (string, error) {
	winner, err := g.GetWinner()
	if err != nil {
		return "", err
	}

	if winner == 0 {
		return "", nil // Draw
	}

	if winner == WHITE {
		return g.GetWhitePlayer(), nil
	}
	return g.GetBlackPlayer(), nil
}

// GetColor returns the color for a given username.
func (g *Game) GetColor(username string) (int, error) {
	if username == g.GetWhitePlayer() {
		return WHITE, nil
	}
	if username == g.GetBlackPlayer() {
		return BLACK, nil
	}
	return 0, fmt.Errorf("username %s not found in game", username)
}

// GetColorAny returns the color for any of the given usernames.
func (g *Game) GetColorAny(usernames []string) (int, error) {
	for _, username := range usernames {
		if color, err := g.GetColor(username); err == nil {
			return color, nil
		}
	}
	return 0, errors.New("none of the usernames found in game")
}

// GetWinner returns the winning color.
func (g *Game) GetWinner() (int, error) {
	result, exists := g.Metadata["Result"]
	if !exists {
		return 0, errors.New("no result in metadata")
	}

	if result == "1/2-1/2" {
		return 0, nil // Draw
	}

	parts := strings.Split(result, "-")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid result format: %s", result)
	}

	blackScore, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse black score: %w", err)
	}

	whiteScore, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse white score: %w", err)
	}

	if blackScore > whiteScore {
		return BLACK, nil
	}
	if whiteScore > blackScore {
		return WHITE, nil
	}
	return 0, nil // Draw
}

// GetBlackScore returns the black score.
func (g *Game) GetBlackScore() (int, error) {
	result, exists := g.Metadata["Result"]
	if !exists {
		return 0, errors.New("no result in metadata")
	}

	if result == "1/2-1/2" {
		return 0, nil
	}

	parts := strings.Split(result, "-")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid result format: %s", result)
	}

	blackScore, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse black score: %w", err)
	}

	whiteScore, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse white score: %w", err)
	}

	if blackScore == whiteScore {
		return 0, nil
	}
	if blackScore > whiteScore {
		return 64 - 2*whiteScore, nil
	}
	return -64 + 2*blackScore, nil
}

// ZipBoardMoves returns an iterator over board-move pairs.
func (g *Game) ZipBoardMoves() []BoardMovePair {
	if len(g.Boards) == 0 || len(g.Moves) == 0 {
		return nil
	}

	pairs := make([]BoardMovePair, len(g.Moves))
	for i := range len(g.Moves) {
		pairs[i] = BoardMovePair{
			Board: g.Boards[i],
			Move:  g.Moves[i],
		}
	}
	return pairs
}

// GetAllChildren returns all possible child boards from all positions in the game.
func (g *Game) GetAllChildren() []Board {
	var allChildren []Board

	for _, board := range g.Boards {
		children := board.GetChildren()
		allChildren = append(allChildren, children...)
	}

	return allChildren
}

// GetNormalizedPositions returns all normalized positions from the game.
func (g *Game) GetNormalizedPositions(addChildren bool) map[NormalizedPosition]bool {
	positions := make(map[NormalizedPosition]bool)

	for _, board := range g.Boards {
		positions[board.Position().Normalized()] = true

		if addChildren {
			for _, childPosition := range board.GetChildPositions() {
				positions[childPosition.Normalized()] = true
			}
		}
	}

	return positions
}

// Equal checks if two games are equal.
func (g *Game) Equal(other *Game) bool {
	if g == nil || other == nil {
		return g == other
	}

	if len(g.Metadata) != len(other.Metadata) {
		return false
	}

	for k, v := range g.Metadata {
		if other.Metadata[k] != v {
			return false
		}
	}

	if len(g.Boards) != len(other.Boards) {
		return false
	}

	for i, board := range g.Boards {
		if !board.Equal(other.Boards[i]) {
			return false
		}
	}

	return true
}

// BoardMovePair represents a board and its corresponding move.
type BoardMovePair struct {
	Board Board
	Move  int
}
