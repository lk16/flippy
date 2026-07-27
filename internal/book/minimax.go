// Package book backfills evaluations for boards below the disc count that's
// ever learned directly (see internal/db), by minimaxing up from the
// evaluations of boards at that disc count.
package book

import "github.com/lk16/flippy/internal/othello"

// noValue is a sentinel score outside the valid -64..64 range, used while
// finding the best of a board's children.
const noValue = -65

// buildCache computes minimax scores, from the perspective of the player to
// move, for every board reachable from root with fewer than leafDiscs
// discs. leaves holds the known scores of normalized boards with exactly
// leafDiscs discs — anything not in leaves is treated as not yet learned.
//
// A board's value is included in the result only if every leafDiscs-disc
// descendant needed to determine it is present in leaves; branches through
// an unlearned leaf are omitted rather than guessed at.
func buildCache(root othello.Board, leafDiscs int, leaves map[othello.Board]int) map[othello.Board]int {
	memo := make(map[othello.Board]cacheEntry)
	visiting := make(map[othello.Board]bool)

	minimaxValue(root, leafDiscs, leaves, memo, visiting)

	result := make(map[othello.Board]int)
	for board, entry := range memo {
		if entry.ok && board.CountDiscs() < leafDiscs {
			result[board] = entry.score
		}
	}
	return result
}

// cacheEntry is a memoized minimax result: a score, and whether it could be
// determined at all.
type cacheEntry struct {
	score int
	ok    bool
}

// minimaxValue returns board's value from the perspective of the player to
// move, memoizing into memo (keyed by normalized board) as it goes.
//
// A board with no legal move is always resolved via minimaxPass first,
// before the leafDiscs boundary is even considered: leaves only ever holds
// boards with a legal move (see othello.PrecomputedBoards12/loader.
// ExtractBoards — edax can't evaluate a no-legal-move position, and its
// value is trivially the negation of whatever it passes into), so a board
// at exactly leafDiscs discs with no legal move would otherwise always miss
// leaves and be reported as undetermined, even though its value is
// perfectly derivable from the position it passes into.
func minimaxValue(
	board othello.Board,
	leafDiscs int,
	leaves map[othello.Board]int,
	memo map[othello.Board]cacheEntry,
	visiting map[othello.Board]bool,
) cacheEntry {
	normalized := board.Normalize().Board()

	if entry, ok := memo[normalized]; ok {
		return entry
	}

	if visiting[normalized] {
		panic("book: cycle detected while computing minimax value")
	}
	visiting[normalized] = true
	defer delete(visiting, normalized)

	var entry cacheEntry
	switch {
	case !board.HasMoves():
		entry = minimaxPass(board, leafDiscs, leaves, memo, visiting)
	case board.CountDiscs() >= leafDiscs:
		score, ok := leaves[normalized]
		entry = cacheEntry{score: score, ok: ok}
	default:
		entry = minimaxChildren(board, leafDiscs, leaves, memo, visiting)
	}

	memo[normalized] = entry
	return entry
}

// minimaxPass handles a board where the player to move has no legal move:
// either the game is over (neither player has a move), or the value is the
// negation of the passed-to position's value.
func minimaxPass(
	board othello.Board,
	leafDiscs int,
	leaves map[othello.Board]int,
	memo map[othello.Board]cacheEntry,
	visiting map[othello.Board]bool,
) cacheEntry {
	passed, err := board.DoMove(othello.PassMove)
	if err != nil {
		panic("book: pass should always be legal when HasMoves is false: " + err.Error())
	}

	if !passed.HasMoves() {
		return cacheEntry{score: board.FinalScore(), ok: true}
	}

	sub := minimaxValue(passed, leafDiscs, leaves, memo, visiting)
	if !sub.ok {
		return cacheEntry{ok: false}
	}
	return cacheEntry{score: -sub.score, ok: true}
}

// minimaxChildren handles a board with legal moves: its value is the best
// of its children's values, each negated since a child is from the
// opponent's perspective.
func minimaxChildren(
	board othello.Board,
	leafDiscs int,
	leaves map[othello.Board]int,
	memo map[othello.Board]cacheEntry,
	visiting map[othello.Board]bool,
) cacheEntry {
	best := noValue
	determined := true

	for _, child := range board.Children() {
		sub := minimaxValue(child, leafDiscs, leaves, memo, visiting)
		if !sub.ok {
			determined = false
			continue
		}
		if score := -sub.score; score > best {
			best = score
		}
	}

	if !determined {
		return cacheEntry{ok: false}
	}
	return cacheEntry{score: best, ok: true}
}
