// Command gen generates the list of all NormalizedBoards with exactly
// targetDiscs discs reachable by legal play from the standard Othello
// starting position. Its output backs othello.PrecomputedBoards12; rerun via
// `go generate ./...` from the internal/othello package.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/lk16/flippy/internal/othello"
)

const targetDiscs = 12

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen <output-file>")
		os.Exit(1)
	}

	found := make(map[string]struct{})
	explore(othello.NewBoardStart(), make(map[othello.Board]bool), found)

	lines := make([]string, 0, len(found))
	for line := range found {
		lines = append(lines, line)
	}
	sort.Strings(lines)

	out := make([]byte, 0, len(lines)*35)
	for _, line := range lines {
		out = append(out, line...)
		out = append(out, '\n')
	}

	if err := os.WriteFile(os.Args[1], out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write failed:", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "wrote %d boards to %s\n", len(lines), os.Args[1])
}

// explore walks every legal line of play from b, recording the normalized
// form of every board it encounters with exactly targetDiscs discs. visited
// memoizes exact (non-normalized) boards already explored, since the same
// board is commonly reached via multiple move orders.
//
// A targetDiscs board with no legal move for the player to move is skipped:
// edax can't evaluate it directly, and its value is trivially the negation
// of the position it passes into, so storing it would only ever leave a
// permanently unlearnable row.
func explore(b othello.Board, visited map[othello.Board]bool, found map[string]struct{}) {
	if visited[b] {
		return
	}
	visited[b] = true

	if b.CountDiscs() == targetDiscs {
		if b.HasMoves() {
			found[b.Normalize().String()] = struct{}{}
		}
		return
	}

	if !b.HasMoves() {
		next, err := b.DoMove(othello.PassMove)
		if err != nil {
			// Neither player has a move: the game ended before reaching
			// targetDiscs discs.
			return
		}
		explore(next, visited, found)
		return
	}

	for _, child := range b.Children() {
		explore(child, visited, found)
	}
}
