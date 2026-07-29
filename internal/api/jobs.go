package api

import (
	"context"
	"fmt"
	"log/slog"

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

// claimJob atomically claims the lowest disc-count/level learnable board not already claimed.
func (s *Server) claimJob(ctx context.Context, workerID string) (Job, bool, error) {
	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	if err != nil {
		return Job{}, false, err
	}

	candidates, err := s.repo.ListLearnable(ctx,
		floor, book.MaxSavableDiscs,
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

		board := candidate.Board.String()

		claimed, err := s.tryClaim(ctx, board, workerID)
		if err != nil {
			return Job{}, false, err
		}
		if claimed {
			// A claim strictly above floor is proof nothing claimable remains at floor in this batch,
			// so it's safe to stop rescanning it; never lower the floor here (see getJobFloor/jobFloorTTL
			// for how a floor stuck above newly-imported boards self-heals).
			if discCount > floor {
				if err := s.setJobFloor(ctx, discCount); err != nil {
					slog.Warn("failed to advance job floor cache", "error", err)
				}
			}
			return Job{Board: candidate.Board, Level: target}, true, nil
		}
	}

	return Job{}, false, nil
}
