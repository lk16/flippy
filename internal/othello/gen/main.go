// Command gen generates the targetDiscs-disc position list backing othello.PrecomputedPositions12.
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
	explore(othello.NewStartPosition(), make(map[othello.Position]bool), found)

	lines := make([]string, 0, len(found))
	for line := range found {
		lines = append(lines, line)
	}
	sort.Strings(lines)

	out := make([]byte, 0, len(lines)*(othello.PositionStringLength+1))
	for _, line := range lines {
		out = append(out, line...)
		out = append(out, '\n')
	}

	if err := os.WriteFile(os.Args[1], out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write failed:", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "wrote %d positions to %s\n", len(lines), os.Args[1])
}

// explore walks every legal line from p, recording targetDiscs-disc positions; a no-legal-move
// position is always passed through rather than recorded, since edax can't evaluate it.
func explore(p othello.Position, visited map[othello.Position]bool, found map[string]struct{}) {
	if visited[p] {
		return
	}
	visited[p] = true

	if !p.HasMoves() {
		next, err := p.DoMove(othello.PassMove)
		if err != nil {
			// Neither player has a move: the game ended before reaching targetDiscs discs.
			return
		}
		explore(next, visited, found)
		return
	}

	if p.CountDiscs() == targetDiscs {
		found[p.Normalize().String()] = struct{}{}
		return
	}

	for _, child := range p.Children() {
		explore(child, visited, found)
	}
}
