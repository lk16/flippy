package othello

import "fmt"

// Game is a sequence of positions from a start position; positions always has one more entry than moves.
type Game struct {
	moves     []int
	positions []Position
	filename  string
	metadata  *GameMetadata
}

// NewGame returns a new game starting from the standard Othello position.
func NewGame() *Game {
	return NewGameWithStart(NewStartPosition())
}

// NewGameWithStart returns a new game starting from start.
func NewGameWithStart(start Position) *Game {
	return &Game{positions: []Position{start}}
}

// NewGameFromMoves returns a new game starting from the standard position with moves played in order.
func NewGameFromMoves(moves []int) (*Game, error) {
	game := NewGame()

	for _, move := range moves {
		if err := game.PushMove(move); err != nil {
			return nil, fmt.Errorf("failed to push move %d: %w", move, err)
		}
	}

	return game, nil
}

// Moves returns the moves played so far, including automatically inserted passes.
func (g *Game) Moves() []int {
	return append([]int(nil), g.moves...)
}

// Position returns the position after all moves played so far.
func (g *Game) Position() Position {
	return g.positions[len(g.positions)-1]
}

// PositionAt returns the position after the first moveIndex moves.
func (g *Game) PositionAt(moveIndex int) Position {
	return g.positions[moveIndex]
}

// Positions returns the full sequence of positions from start to the current one, inclusive.
func (g *Game) Positions() []Position {
	return append([]Position(nil), g.positions...)
}

// Filename returns the path of the file the game was loaded from, or "" if none.
func (g *Game) Filename() string {
	return g.filename
}

// Metadata returns the game's metadata, or nil if it has none.
func (g *Game) Metadata() *GameMetadata {
	if g.metadata == nil {
		return nil
	}

	metadata := *g.metadata
	return &metadata
}

// PushMove plays move, automatically appending a pass if the resulting position has no legal move.
func (g *Game) PushMove(move int) error {
	if n := len(g.moves); n > 0 && g.moves[n-1] == PassMove && move == PassMove {
		return nil
	}

	next, err := g.Position().DoMove(move)
	if err != nil {
		return fmt.Errorf("invalid move: %d", move)
	}

	g.moves = append(g.moves, move)
	g.positions = append(g.positions, next)

	if move == PassMove || next.HasMoves() {
		return nil
	}

	// next has no moves, so passing is always legal here.
	passed, _ := next.DoMove(PassMove)
	if passed.HasMoves() {
		g.moves = append(g.moves, PassMove)
		g.positions = append(g.positions, passed)
	}

	return nil
}

// PopMove undoes the last move, and the move before it too if it was an automatically inserted pass.
func (g *Game) PopMove() {
	if len(g.moves) == 0 {
		return
	}

	n := 1
	if len(g.moves) >= 2 && g.moves[len(g.moves)-1] == PassMove {
		n = 2
	}

	g.moves = g.moves[:len(g.moves)-n]
	g.positions = g.positions[:len(g.positions)-n]
}
