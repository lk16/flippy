package api

import "github.com/lk16/flippy/internal/book"

// TargetLevel returns the edax search level to use for a board with discCount discs. The 12-disc
// boards are searched deepest, since every board below them is backfilled by minimaxing up from their
// evaluations; boards beyond 12 discs are only ever looked at directly, so they use a shallower level.
func TargetLevel(discCount int) int {
	if discCount > book.LeafDiscs {
		return 16
	}
	return 24
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
