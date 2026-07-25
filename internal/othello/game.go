package othello

import "fmt"

// Game is a sequence of boards produced by playing moves from a start
// board. boards always has one more entry than moves: boards[i] is the
// board after the first i moves, and boards[0] is the start board. Passes
// are usually inserted automatically by PushMove; see there for details.
type Game struct {
	moves  []int
	boards []Board
}

// NewGame returns a new game starting from the standard Othello position.
func NewGame() *Game {
	return NewGameWithStart(NewBoardStart())
}

// NewGameWithStart returns a new game starting from start.
func NewGameWithStart(start Board) *Game {
	return &Game{boards: []Board{start}}
}

// NewGameFromMoves returns a new game starting from the standard position
// with moves played on it in order.
func NewGameFromMoves(moves []int) (*Game, error) {
	game := NewGame()

	for _, move := range moves {
		if err := game.PushMove(move); err != nil {
			return nil, fmt.Errorf("failed to push move %d: %w", move, err)
		}
	}

	return game, nil
}

// Moves returns the moves played so far, including automatically inserted
// passes.
func (g *Game) Moves() []int {
	return append([]int(nil), g.moves...)
}

// Board returns the board after all moves played so far.
func (g *Game) Board() Board {
	return g.boards[len(g.boards)-1]
}

// BoardAt returns the board after the first moveIndex moves.
func (g *Game) BoardAt(moveIndex int) Board {
	return g.boards[moveIndex]
}

// Boards returns the full sequence of boards from start to the current
// board, inclusive.
func (g *Game) Boards() []Board {
	return append([]Board(nil), g.boards...)
}

// PushMove plays move on the current board. If the resulting board has no
// legal move for the player to move but their opponent does, a pass is
// appended automatically. Playing a pass right after another pass is a
// no-op, since two passes in a row would mean the game already ended.
func (g *Game) PushMove(move int) error {
	if n := len(g.moves); n > 0 && g.moves[n-1] == PassMove && move == PassMove {
		return nil
	}

	next, err := g.Board().DoMove(move)
	if err != nil {
		return fmt.Errorf("invalid move: %d", move)
	}

	g.moves = append(g.moves, move)
	g.boards = append(g.boards, next)

	if move == PassMove || next.HasMoves() {
		return nil
	}

	// next has no moves, so passing is always legal here.
	passed, _ := next.DoMove(PassMove)
	if passed.HasMoves() {
		g.moves = append(g.moves, PassMove)
		g.boards = append(g.boards, passed)
	}

	return nil
}

// PopMove undoes the last move. If it was an automatically inserted pass,
// the move before it is undone too, so the game never ends up on a board
// with no legal move for the player to move.
func (g *Game) PopMove() {
	if len(g.moves) == 0 {
		return
	}

	n := 1
	if g.moves[len(g.moves)-1] == PassMove {
		n = 2
	}

	g.moves = g.moves[:len(g.moves)-n]
	g.boards = g.boards[:len(g.boards)-n]
}
