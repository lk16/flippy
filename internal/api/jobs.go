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

// claimJobs atomically claims up to count of the lowest disc-count/level learnable boards not already
// claimed; it may return fewer than count if there aren't enough candidates, including zero.
// Priority-queue boards (from interactive analysis requests) are drained first.
func (s *Server) claimJobs(ctx context.Context, workerID string, count int) ([]Job, error) {
	var jobs []Job

	// Drain up to count entries from the priority queue before falling back to ListLearnable.
	priorityEntries, err := s.dequeuePriority(ctx, count)
	if err != nil {
		return nil, fmt.Errorf("failed to drain priority queue: %w", err)
	}

	for _, entry := range priorityEntries {
		if len(jobs) >= count {
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
			return nil, err
		}
		if !claimed {
			continue
		}

		if err := s.setPriorityClaim(ctx, entry.Board); err != nil {
			slog.Warn("failed to set priority claim marker", "board", entry.Board, "error", err)
		}

		jobs = append(jobs, Job{Board: normalized, Level: entry.Level})
	}

	// Fall back to ListLearnable for whatever slots remain.
	remaining := count - len(jobs)
	if remaining == 0 {
		return jobs, nil
	}

	floor, err := s.getJobFloor(ctx, book.LeafDiscs)
	if err != nil {
		return nil, err
	}

	candidates, err := s.repo.ListLearnable(ctx,
		floor, book.MaxSavableDiscs,
		book.LeafDiscs, TargetLevel(book.LeafDiscs), TargetLevel(book.LeafDiscs+1),
		jobCandidateBatch,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list candidate boards: %w", err)
	}

	maxClaimedDiscs := floor

	for _, candidate := range candidates {
		if len(jobs) >= count {
			break
		}

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
			return nil, err
		}
		if !claimed {
			continue
		}

		jobs = append(jobs, Job{Board: candidate.Board, Level: target})
		if discCount > maxClaimedDiscs {
			maxClaimedDiscs = discCount
		}
	}

	// A claim strictly above floor is proof nothing claimable remains at floor in this batch, so it's
	// safe to stop rescanning it; never lower the floor here (see getJobFloor/jobFloorTTL for how a
	// floor stuck above newly-imported boards self-heals).
	if maxClaimedDiscs > floor {
		if err := s.setJobFloor(ctx, maxClaimedDiscs); err != nil {
			slog.Warn("failed to advance job floor cache", "error", err)
		}
	}

	return jobs, nil
}
