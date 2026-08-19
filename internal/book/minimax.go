// Package book backfills evaluations for boards below the disc count learned directly, by minimaxing
// up from the evaluations of boards at that disc count.
package book

import "github.com/lk16/flippy/internal/othello"

// noValue is a sentinel score below the valid -64..64 range.
const noValue = -65

// buildCache computes minimax scores for every board below leafDiscs reachable from root, omitting
// any board with an unlearned leafDiscs-disc descendant rather than guessing at it.
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

// cacheEntry is a memoized minimax result: a score, and whether it could be determined at all.
type cacheEntry struct {
	score int
	ok    bool
}

// minimaxValue returns board's value, memoizing into memo. A no-legal-move board is always resolved
// via minimaxPass first since leaves never holds such boards (edax can't evaluate them).
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

// minimaxPass handles a forced pass: the game-over final score, or the negated passed-to value.
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

// minimaxChildren returns the best negated value among board's children.
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
