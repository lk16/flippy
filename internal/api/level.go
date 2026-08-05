package api

import "github.com/lk16/flippy/internal/book"

// TargetLevel returns the edax search level to use for a board with discCount discs. Deeper boards
// get shallower searches to keep evaluation time roughly bounded as the search tree widens.
func TargetLevel(discCount int) int {
	switch {
	case discCount <= 16:
		return 32
	case discCount <= 20:
		return 30
	default:
		return 28
	}
}

// EffectiveTargetLevel returns the target level for a board at the given disc count, capping at
// TargetLevel(MaxSavableDiscs) for boards that exceed that count and are not persisted to the DB.
func EffectiveTargetLevel(discCount int) int {
	if discCount > book.MaxSavableDiscs {
		discCount = book.MaxSavableDiscs
	}
	return TargetLevel(discCount)
}

// PriorityLevel is the edax search depth used for the first interactive priority-queue request.
// Deliberately lighter than TargetLevel so the worker can respond quickly; the frontend then
// increments by 2 per round until EffectiveTargetLevel is reached.
const PriorityLevel = 10
