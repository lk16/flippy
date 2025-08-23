package models

import (
	"fmt"
	"os"
	"strings"
)

// Game represents an Othello game, either complete or in progress.
type Game struct {
	// filename is the PGN file path or empty if created manually
	filename string

	// metadata is the PGN metadata
	metadata *GameMetadata

	// moves is the list of moves in the game. Pass moves are added automatically.
	moves []int
}

// NewGame creates a new empty game.
func NewGame() *Game {
	return &Game{
		metadata: &GameMetadata{},
		moves:    make([]int, 0),
	}
}

// NewGameFromPGN creates a new game from a PGN file.
func NewGameFromPGN(filename string) (*Game, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	metadataRowCount := 0
	for _, line := range lines {
		if !strings.HasPrefix(line, "[") {
			break
		}
		metadataRowCount++
	}

	metadata, err := parseMetadata(lines[:metadataRowCount], filename)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	moves, err := parsePgnMoves(lines[metadataRowCount:])
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

// NewGameFromMoves creates a new game from a list of moves.
func NewGameFromMoves(moves []int) (*Game, error) {
	game := NewGame()

	for _, move := range moves {
		if err := game.PushMove(move); err != nil {
			return nil, fmt.Errorf("failed to push move: %w", err)
		}
	}

	return game, nil
}

func parsePgnMoves(lines []string) ([]int, error) {
	moves := make([]int, 0)

	for _, line := range lines {
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

			moves = append(moves, move)
		}
	}

	return moves, nil
}

// MetaData returns a copy of the game metadata.
func (g *Game) MetaData() *GameMetadata {
	if g.metadata == nil {
		return nil
	}

	metadata := *g.metadata
	return &metadata
}

// GetNormalizedPositionsWithChildren returns all normalized positions from the game with their children.
func (g *Game) GetNormalizedPositionsWithChildren() map[NormalizedPosition]bool {
	positions := make(map[NormalizedPosition]bool)

	for moveIndex := range len(g.moves) + 1 {
		board := g.GetBoard(moveIndex)
		positions[board.Position().Normalized()] = true

		for _, childPosition := range board.GetChildPositions() {
			positions[childPosition.Normalized()] = true
		}
	}

	return positions
}

// LastBoard returns the last board in the game.
func (g *Game) LastBoard() Board {
	return g.GetBoard(len(g.moves))
}

// GetBoard returns the board after doing the moves up to the given move index.
func (g *Game) GetBoard(moveIndex int) Board {
	board := NewBoardStart()

	for i := range moveIndex {
		board = board.DoMove(g.moves[i])
	}

	return board
}

// PushMove appends a move to the game.
func (g *Game) PushMove(move int) error {
	moveCount := len(g.moves)

	if moveCount > 0 {
		lastMove := g.moves[moveCount-1]

		// Prevent double pass.
		if lastMove == PassMove && move == PassMove {
			return nil
		}
	}

	board := g.LastBoard()
	if !board.IsValidMove(move) {
		return fmt.Errorf("invalid move: %d", move)
	}

	g.moves = append(g.moves, move)

	// Try adding a pass move if we didn't pass last move.
	if move != PassMove {
		board = g.LastBoard()
		passed := board.DoMove(PassMove)

		// Add pass move if current player doesn't have moves but opponent does.
		if !board.HasMoves() && passed.HasMoves() {
			g.moves = append(g.moves, PassMove)
		}
	}

	return nil
}

// PopMove undoes the last move.
func (g *Game) PopMove() {
	if len(g.moves) == 0 {
		return
	}

	g.moves = g.moves[:len(g.moves)-1]

	if len(g.moves) == 0 {
		return
	}

	if g.moves[len(g.moves)-1] == PassMove {
		g.moves = g.moves[:len(g.moves)-1]
	}
}

// GetFilename returns the filename of the game.
func (g *Game) GetFilename() string {
	return g.filename
}
