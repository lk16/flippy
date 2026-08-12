package api

import (
	"slices"

	"github.com/lk16/flippy/internal/book"
)

// TargetLevelTier maps an upper disc-count bound to the edax search level boards up to that count
// get. Tiers are ordered by MaxDiscs; the last one is a catch-all for the rest of the board.
type TargetLevelTier struct {
	MaxDiscs int `json:"max_discs"`
	Level    int `json:"level"`
}

// targetLevelTiers is the single source of truth for TargetLevel, also served to the frontend
// verbatim by handleLevelConfig so it computes the same targets the server enforces.
// Levels match the deepest search the pre-rewrite archive holds at each disc count, so an imported
// row lands exactly at target rather than above or below it (see docs/next-steps.md).
var targetLevelTiers = []TargetLevelTier{
	{MaxDiscs: 13, Level: 40},
	{MaxDiscs: 16, Level: 36},
	{MaxDiscs: 20, Level: 34},
	{MaxDiscs: 64, Level: 32},
}

// TargetLevelTiers returns a copy of the disc-count tiers TargetLevel is defined by.
func TargetLevelTiers() []TargetLevelTier {
	return slices.Clone(targetLevelTiers)
}

// TargetLevel returns the edax search level to use for a board with discCount discs. Deeper boards
// get shallower searches to keep evaluation time roughly bounded as the search tree widens.
func TargetLevel(discCount int) int {
	for _, tier := range targetLevelTiers {
		if discCount <= tier.MaxDiscs {
			return tier.Level
		}
	}
	return targetLevelTiers[len(targetLevelTiers)-1].Level
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
