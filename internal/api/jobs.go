package api

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
)

// jobCandidateBatch is how many candidate boards are fetched per claim attempt.
const jobCandidateBatch = 50

// Job is a board a worker should evaluate, and the level to search it at.
type Job struct {
	Board othello.NormalizedBoard
	Level int
}

// claimJob atomically claims one board for workerID: the oldest claimable priority-queue board
// (from interactive analysis requests) if any, else the lowest disc-count/level learnable board.
// ok is false when nothing is claimable.
func (s *Server) claimJob(ctx context.Context, workerID string) (job Job, ok bool, err error) {
	// Drain the priority queue before falling back to ListLearnable.
	for {
		entry, found, err := s.dequeuePriority(ctx)
		if err != nil {
			return Job{}, false, fmt.Errorf("failed to drain priority queue: %w", err)
		}
		if !found {
			break
		}

		board, err := othello.ParseBoard(entry.Board)
		if err != nil {
			continue
		}

		// edax crashes on a position with no legal move.
		if !board.HasMoves() {
			continue
		}

		normalized, err := othello.NewNormalizedBoard(board)
		if err != nil {
			// Board from queue wasn't normalized; skip rather than corrupt the claim key.
			continue
		}

		claimed, err := s.tryClaim(ctx, entry.Board, workerID)
		if err != nil {
			return Job{}, false, err
		}
		if !claimed {
			continue
		}

		if err := s.setPriorityClaim(ctx, entry.Board); err != nil {
			log.Printf("failed to set priority claim marker for %s: %v", entry.Board, err)
		}

		return Job{Board: normalized, Level: entry.Level}, true, nil
	}

	candidates, err := s.repo.ListLearnable(ctx,
		s.jobFloor(ctx), book.MaxSavableDiscs,
		book.LeafDiscs, TargetLevel(book.LeafDiscs), TargetLevel(book.LeafDiscs+1),
		jobCandidateBatch,
	)
	if err != nil {
		return Job{}, false, fmt.Errorf("failed to list candidate boards: %w", err)
	}

	for _, candidate := range candidates {
		// edax crashes on a position with no legal move.
		if !candidate.Board.HasMoves() {
			continue
		}

		discCount := candidate.Board.CountDiscs()
		target := TargetLevel(discCount)
		if candidate.Evaluation.Level >= target {
			continue
		}

		claimed, err := s.tryClaim(ctx, candidate.Board.String(), workerID)
		if err != nil {
			return Job{}, false, err
		}
		if !claimed {
			continue
		}

		return Job{Board: candidate.Board, Level: target}, true, nil
	}

	return Job{}, false, nil
}

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
