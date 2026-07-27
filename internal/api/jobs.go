package api

import (
	"context"
	"fmt"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
)

const (
	// jobCandidateBatch is how many candidate boards are fetched per claim
	// attempt. It must be large enough that, even with several workers
	// racing for jobs, at least one candidate in the batch is still free.
	jobCandidateBatch = 50
)

// Job is a board a worker should evaluate, and the level to search it at.
type Job struct {
	Board othello.NormalizedBoard
	Level int
}

// claimJob finds and atomically claims the next job for workerID: the
// learnable board with the lowest disc count, then lowest level, that isn't
// already claimed by another worker. It reports false, with no error, if no
// job is currently available.
func (s *Server) claimJob(ctx context.Context, workerID string) (Job, bool, error) {
	candidates, err := s.repo.ListLearnable(ctx,
		book.LeafDiscs, book.MaxSavableDiscs,
		TargetLevel(book.LeafDiscs), TargetLevel(book.LeafDiscs+1),
		jobCandidateBatch,
	)
	if err != nil {
		return Job{}, false, fmt.Errorf("failed to list candidate boards: %w", err)
	}

	for _, candidate := range candidates {
		// edax crashes when asked to evaluate a position with no legal
		// move; such boards are never valid jobs.
		if !candidate.Board.HasMoves() {
			continue
		}

		target := TargetLevel(candidate.Board.CountDiscs())
		if candidate.Evaluation.Level >= target {
			continue
		}

		board := candidate.Board.String()

		claimed, err := s.tryClaim(ctx, board, workerID)
		if err != nil {
			return Job{}, false, err
		}
		if claimed {
			return Job{Board: candidate.Board, Level: target}, true, nil
		}
	}

	return Job{}, false, nil
}
