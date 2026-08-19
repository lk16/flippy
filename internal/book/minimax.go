// Package book backfills evaluations for positions below the disc count learned directly, by
// minimaxing up from the evaluations of positions at that disc count.
package book

import "github.com/lk16/flippy/internal/othello"

// noValue is a sentinel score below the valid -64..64 range.
const noValue = -65

// buildCache computes minimax scores for every position below leafDiscs reachable from root,
// omitting any position with an unlearned leafDiscs-disc descendant rather than guessing at it.
func buildCache(root othello.Position, leafDiscs int, leaves map[othello.Position]int) map[othello.Position]int {
	memo := make(map[othello.Position]cacheEntry)
	visiting := make(map[othello.Position]bool)

	minimaxValue(root, leafDiscs, leaves, memo, visiting)

	result := make(map[othello.Position]int)
	for position, entry := range memo {
		if entry.ok && position.CountDiscs() < leafDiscs {
			result[position] = entry.score
		}
	}
	return result
}

// cacheEntry is a memoized minimax result: a score, and whether it could be determined at all.
type cacheEntry struct {
	score int
	ok    bool
}

// minimaxValue returns position's value, memoizing into memo. A no-legal-move position is always
// resolved via minimaxPass first since leaves never holds such positions (edax can't evaluate them).
func minimaxValue(
	position othello.Position,
	leafDiscs int,
	leaves map[othello.Position]int,
	memo map[othello.Position]cacheEntry,
	visiting map[othello.Position]bool,
) cacheEntry {
	normalized := position.Normalize().Position()

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
	case !position.HasMoves():
		entry = minimaxPass(position, leafDiscs, leaves, memo, visiting)
	case position.CountDiscs() >= leafDiscs:
		score, ok := leaves[normalized]
		entry = cacheEntry{score: score, ok: ok}
	default:
		entry = minimaxChildren(position, leafDiscs, leaves, memo, visiting)
	}

	memo[normalized] = entry
	return entry
}

// minimaxPass handles a forced pass: the game-over final score, or the negated passed-to value.
func minimaxPass(
	position othello.Position,
	leafDiscs int,
	leaves map[othello.Position]int,
	memo map[othello.Position]cacheEntry,
	visiting map[othello.Position]bool,
) cacheEntry {
	passed, err := position.DoMove(othello.PassMove)
	if err != nil {
		panic("book: pass should always be legal when HasMoves is false: " + err.Error())
	}

	if !passed.HasMoves() {
		return cacheEntry{score: position.FinalScore(), ok: true}
	}

	sub := minimaxValue(passed, leafDiscs, leaves, memo, visiting)
	if !sub.ok {
		return cacheEntry{ok: false}
	}
	return cacheEntry{score: -sub.score, ok: true}
}

// minimaxChildren returns the best negated value among position's children.
func minimaxChildren(
	position othello.Position,
	leafDiscs int,
	leaves map[othello.Position]int,
	memo map[othello.Position]cacheEntry,
	visiting map[othello.Position]bool,
) cacheEntry {
	best := noValue
	determined := true

	for _, child := range position.Children() {
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
