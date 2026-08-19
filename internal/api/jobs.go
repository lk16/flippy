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
			slog.Warn("failed to set priority claim marker", "board", entry.Board, "error", err)
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
