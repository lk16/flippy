// Command gen generates the reachable targetDiscs-disc board list backing othello.PrecomputedBoards12.
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

	out := make([]byte, 0, len(lines)*(othello.BoardStringLength+1))
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

// explore walks every legal line from b, recording targetDiscs-disc boards; a no-legal-move board is
// always passed through rather than recorded, since edax can't evaluate it.
func explore(b othello.Board, visited map[othello.Board]bool, found map[string]struct{}) {
	if visited[b] {
		return
	}
	visited[b] = true

	if !b.HasMoves() {
		next, err := b.DoMove(othello.PassMove)
		if err != nil {
			// Neither player has a move: the game ended before reaching targetDiscs discs.
			return
		}
		explore(next, visited, found)
		return
	}

	if b.CountDiscs() == targetDiscs {
		found[b.Normalize().String()] = struct{}{}
		return
	}

	for _, child := range b.Children() {
		explore(child, visited, found)
	}
}
