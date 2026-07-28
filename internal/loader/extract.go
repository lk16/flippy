package loader

import (
	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
)

// ExtractBoards returns the deduplicated, savable NormalizedBoards from games: every played-line board
// plus its one-ply children, for broader book coverage than just the moves actually played.
func ExtractBoards(games []*othello.Game) []othello.NormalizedBoard {
	seen := make(map[othello.Board]struct{})
	var boards []othello.NormalizedBoard

	add := func(b othello.Board) {
		if !isSavable(b) {
			return
		}

		normalized := b.Normalize()
		if _, ok := seen[normalized.Board()]; ok {
			return
		}

		seen[normalized.Board()] = struct{}{}
		boards = append(boards, normalized)
	}

	for _, game := range games {
		for _, b := range game.Boards() {
			add(b)
			for _, child := range b.Children() {
				add(child)
			}
		}
	}

	return boards
}

func isSavable(b othello.Board) bool {
	if !b.HasMoves() {
		return false
	}

	discs := b.CountDiscs()
	return discs >= book.LeafDiscs && discs <= book.MaxSavableDiscs
}
