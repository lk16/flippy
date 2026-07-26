package loader

import (
	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
)

// ExtractBoards returns the deduplicated set of NormalizedBoards worth
// adding to the DB from games: every board on each game's played line, plus
// every one-ply legal child of those boards (so an import gives broader book
// coverage than just the moves actually played, not only the played line
// itself).
//
// A board is kept only if the player to move has a legal move and its disc
// count is in [book.LeafDiscs, book.MaxSavableDiscs] — the same range
// AddBoards' consumers (job claiming, the startup minimax cache) care
// about; anything outside it would never be looked at again.
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
