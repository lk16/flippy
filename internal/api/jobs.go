package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

// jobCandidateBatch is how many candidate positions one refill of the shared buffer reads. Workers
// pop from that buffer instead of each scanning the same ordering, so this sets how many claims a
// single DB query serves, not how many workers can be served at once.
const jobCandidateBatch = 2000

// jobClaimAttempts bounds how many buffered candidates one claim walks past before giving up and
// letting the worker retry. Only entries stale enough to be unusable are walked past, so this need
// only absorb a burst of them, and the loop must end: a refill can return candidates that every
// pass then rejects.
const jobClaimAttempts = 16

// Job is a position a worker should evaluate, and the level to search it at.
type Job struct {
	Position othello.NormalizedPosition
	Level    int
}

// claimJob atomically claims one position for workerID, highest tier first: a position requested
// from the frontend (the priority queue), then an unlearned book position, then one below its
// target level. The first two are searched at UnlearnedLevel, the third at its target. ok is false
// when nothing is claimable.
func (s *Server) claimJob(ctx context.Context, workerID string) (job Job, ok bool, err error) {
	// Drain the priority queue before falling back to the candidate buffer.
	for {
		entry, found, err := s.dequeuePriority(ctx)
		if err != nil {
			return Job{}, false, fmt.Errorf("failed to drain priority queue: %w", err)
		}
		if !found {
			break
		}

		position, err := othello.ParsePosition(entry.Position)
		if err != nil {
			continue
		}

		// edax crashes on a position with no legal move.
		if !position.HasMoves() {
			continue
		}

		normalized, err := othello.NewNormalizedPosition(position)
		if err != nil {
			// Position from queue wasn't normalized; skip rather than corrupt the claim key.
			continue
		}

		claimed, err := s.tryClaim(ctx, entry.Position, workerID)
		if err != nil {
			return Job{}, false, err
		}
		if !claimed {
			continue
		}

		if err := s.setPriorityClaim(ctx, entry.Position); err != nil {
			log.Printf("failed to set priority claim marker for %s: %v", entry.Position, err)
		}

		return Job{Position: normalized, Level: entry.Level}, true, nil
	}

	return s.claimBufferedJob(ctx, workerID)
}

// claimBufferedJob claims one candidate from the shared buffer, refilling it when empty, and picks
// the level from the tier that buffered the entry: UnlearnedLevel for the unlearned tier, the
// position's target for the partially learned one. Entries are re-checked against the DB because a
// sweep may have buffered them well before this pop: the position can have been learned, or claimed
// through the priority queue, in between. An entry whose row no longer matches its tier is stale --
// in particular, an unlearned entry learned meanwhile is skipped rather than handed out as a
// target-level job, which would jump the partially learned tier ahead of remaining unlearned rows.
func (s *Server) claimBufferedJob(ctx context.Context, workerID string) (job Job, ok bool, err error) {
	// Walked-past candidates, by why: another worker holds the claim, or the buffered entry no
	// longer describes work. Logged only when there were any, so a clean claim stays silent.
	var takenByOthers, stale int

	for range jobClaimAttempts {
		position, tier, found, err := s.popJobCandidate(ctx)
		if err != nil {
			return Job{}, false, err
		}
		if !found {
			buffered, err := s.refillJobBuffer(ctx)
			if err != nil {
				return Job{}, false, err
			}
			if buffered == 0 {
				logClaimAttempts("", takenByOthers, stale)
				return Job{}, false, nil
			}
			continue
		}

		// edax crashes on a position with no legal move.
		if !position.HasMoves() {
			stale++
			continue
		}

		discCount := position.CountDiscs()
		target := TargetLevel(discCount)

		eval, err := s.repo.GetPosition(ctx, position.Position())
		if errors.Is(err, db.ErrPositionNotFound) {
			stale++
			continue
		}
		if err != nil {
			return Job{}, false, fmt.Errorf("failed to check candidate position: %w", err)
		}

		var level int
		switch tier {
		case tierUnlearned:
			if eval.IsLearned() {
				stale++
				continue
			}
			level = UnlearnedLevel(discCount)
		default:
			if !eval.IsLearned() || eval.Level >= target {
				stale++
				continue
			}
			level = target
		}

		claimed, err := s.tryClaim(ctx, position.String(), workerID)
		if err != nil {
			return Job{}, false, err
		}
		if !claimed {
			takenByOthers++
			continue
		}

		logClaimAttempts(position.String(), takenByOthers, stale)
		return Job{Position: position, Level: level}, true, nil
	}

	logClaimAttempts("", takenByOthers, stale)
	return Job{}, false, nil
}

// logClaimAttempts reports how many buffered candidates one claim had to walk past before taking
// position (empty for none): the two counts say whether the buffer is contended or just stale.
func logClaimAttempts(position string, takenByOthers, stale int) {
	if takenByOthers == 0 && stale == 0 {
		return
	}

	taken := position
	if taken == "" {
		taken = "nothing"
	}

	log.Printf("job buffer: took %s past %d already-claimed and %d stale candidates",
		taken, takenByOthers, stale)
}

// TargetLevelTier maps an upper disc-count bound to the edax search level positions up to that count
// get. Tiers are ordered by MaxDiscs; the last one is a catch-all for the rest of the position.
type TargetLevelTier struct {
	MaxDiscs int `json:"max_discs"`
	Level    int `json:"level"`
}

// targetLevelTiers is the single source of truth for TargetLevel, served verbatim to the frontend
// by handleLevelConfig. Levels match the deepest search the archived book holds per disc count, so
// imported rows land exactly at target.
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

// tierLevel returns the level targetLevelTiers assigns to discCount, before parity alignment.
func tierLevel(discCount int) int {
	for _, tier := range targetLevelTiers {
		if discCount <= tier.MaxDiscs {
			return tier.Level
		}
	}
	return targetLevelTiers[len(targetLevelTiers)-1].Level
}

// TargetLevel returns the edax search level for a position with discCount discs; deeper positions get
// shallower searches to keep evaluation time roughly bounded. Parity-aligned, so adjacent plies are
// never both searched at the same depth parity (see edax.AlignLevel).
func TargetLevel(discCount int) int {
	return edax.AlignLevel(discCount, tierLevel(discCount))
}

// EffectiveTargetLevel returns the target level for a position at the given disc count, capping at
// the tier of MaxSavableDiscs for positions that exceed that count and are not persisted to the DB.
// Only the tier lookup is capped: parity alignment uses the position's real disc count.
func EffectiveTargetLevel(discCount int) int {
	return edax.AlignLevel(discCount, tierLevel(min(discCount, book.MaxSavableDiscs)))
}

// ParityBumpDiscs returns the disc counts whose EffectiveTargetLevel is one above their tier's
// level, so the frontend can reproduce the target from the tier table alone.
func ParityBumpDiscs() []int {
	var discCounts []int
	for discCount := range 65 {
		if EffectiveTargetLevel(discCount) != tierLevel(min(discCount, book.MaxSavableDiscs)) {
			discCounts = append(discCounts, discCount)
		}
	}
	return discCounts
}

// PriorityLevel is the level an interactive analysis request and an unlearned book row are searched
// at -- the top two of the three job tiers. Light, so a worker answers quickly, but deep enough that
// the result is worth a row; the position is re-searched at its target level afterwards, as a
// below-target row. Aligned per position by UnlearnedLevel before it becomes a search level.
const PriorityLevel = 16

// UnlearnedLevel returns the level a position with discCount discs is first searched at:
// PriorityLevel, parity-aligned. So an interactive request and the book row it leaves behind
// describe the same search.
func UnlearnedLevel(discCount int) int {
	return edax.AlignLevel(discCount, PriorityLevel)
}
