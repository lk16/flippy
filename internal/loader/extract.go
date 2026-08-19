package loader

import (
	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
)

// ExtractPositions returns the deduplicated, savable NormalizedPositions from games: every
// played-line position plus its one-ply children, for broader book coverage than just the moves
// actually played.
func ExtractPositions(games []*othello.Game) []othello.NormalizedPosition {
	seen := make(map[othello.Position]struct{})
	var positions []othello.NormalizedPosition

	add := func(b othello.Position) {
		if !isSavable(b) {
			return
		}

		normalized := b.Normalize()
		if _, ok := seen[normalized.Position()]; ok {
			return
		}

		seen[normalized.Position()] = struct{}{}
		positions = append(positions, normalized)
	}

	for _, game := range games {
		for _, b := range game.Positions() {
			add(b)
			for _, child := range b.Children() {
				add(child)
			}
		}
	}

	return positions
}

func isSavable(b othello.Position) bool {
	if !b.HasMoves() {
		return false
	}

	discs := b.CountDiscs()
	return discs >= book.LeafDiscs && discs <= book.MaxSavableDiscs
}
