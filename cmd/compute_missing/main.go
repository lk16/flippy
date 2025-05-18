package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	minDiscs  = 4
	maxDiscs  = 64
	batchSize = 20
)

func main() {
	discCount := flag.Int("n", 0, "Maximum disc count to compute (must be between 4 and 64)")
	flag.Parse()

	if *discCount < minDiscs || *discCount > maxDiscs {
		slog.Error("Invalid disc count", "min", minDiscs, "max", maxDiscs, "got", *discCount)
		os.Exit(1)
	}

	cfg := config.LoadLearnClientConfig()

	client, err := book.NewAPIClient(cfg)
	if err != nil {
		slog.Error("Failed to create API client", "error", err)
		os.Exit(1)
	}

	edaxManager := edax.NewManager()

	for discs := minDiscs; discs <= *discCount; discs++ {
		checkDiscCount(discs, client, edaxManager)
	}
}

func checkDiscCount(discCount int, client *book.APIClient, edaxManager *edax.Manager) {
	// Generate all possible allPositions for this disc count
	allPositions := generateAllPositions(discCount)

	// check which positions are already in the database
	foundPositions := lookupPositions(allPositions, client)

	missingPositions := make([]models.NormalizedPosition, 0)

	for _, pos := range allPositions {
		if _, ok := foundPositions[pos]; ok {
			continue
		}
		missingPositions = append(missingPositions, pos)
	}

	slog.Info(
		"Checking missing positions",
		"disc_count",
		discCount,
		"all",
		len(allPositions),
		"found",
		len(foundPositions),
		"missing",
		len(missingPositions),
	)

	// Compute missing positions at minimum book learn level
	err := computeMissingPositions(discCount, missingPositions, config.MinBookLearnLevel, edaxManager, client)
	if err != nil {
		slog.Error("Failed to compute missing positions", "error", err)
		return
	}
}

func lookupPositions(
	positions []models.NormalizedPosition,
	client *book.APIClient,
) map[models.NormalizedPosition]struct{} {
	// Look up existing positions in batches
	foundPositions := make(map[models.NormalizedPosition]struct{})
	for i := 0; i < len(positions); i += batchSize {
		end := i + batchSize
		if end > len(positions) {
			end = len(positions)
		}

		batch := positions[i:end]

		evaluations, err := client.LookupPositions(batch)
		if err != nil {
			slog.Error("Failed to lookup positions", "error", err, "batch_start", i, "batch_end", end)
			continue
		}

		for _, eval := range evaluations {
			foundPositions[eval.Position] = struct{}{}
		}
	}

	return foundPositions
}

func generateAllPositions(discCount int) []models.NormalizedPosition {
	// positions is effectively a set of positions.
	positions := make(map[models.NormalizedPosition]struct{})

	// Start with the start position, which is the only position with disc count 4
	startPos := models.NewPositionStart().Normalized()
	positions[startPos] = struct{}{}

	// For each disc count from 5 to the target
	for discs := 5; discs <= discCount; discs++ {
		// We don't need to compute positions that will not be saved in the database
		if discs > config.MaxBookSavableDiscs {
			break
		}

		// Create a new set for this disc count
		newPositions := make(map[models.NormalizedPosition]struct{})

		// For each position in the current set
		for nPos := range positions {
			children := nPos.Position().GetChildren()

			// If nPos has no children, we need to pass.
			// Add passed position only if it has children.
			if len(children) == 0 {
				childPassed := nPos.Position().DoMove(models.PassMove)

				children = childPassed.GetChildren()
				if len(children) == 0 {
					continue
				}
			}

			// Add children or passed children
			for _, child := range children {
				newPositions[child.Normalized()] = struct{}{}
			}
		}

		// Update positions for next iteration
		positions = newPositions
	}

	positionsSlice := make([]models.NormalizedPosition, 0, len(positions))
	for pos := range positions {
		positionsSlice = append(positionsSlice, pos)
	}
	return positionsSlice
}

func computeMissingPositions(
	discCount int,
	positions []models.NormalizedPosition,
	level int,
	edaxManager *edax.Manager,
	client *book.APIClient,
) error {
	evaluations := make([]models.Evaluation, 0)

	for i, pos := range positions {
		if i%100 == 0 {
			slog.Info("Computing position", "disc_count", discCount, "index", i, "max_index", len(positions)-1)
		}

		// If the position has no moves, we need to pass
		if !pos.HasMoves() {
			pos = pos.Position().DoMove(models.PassMove).Normalized()
		}

		job := models.Job{
			Position: pos,
			Level:    level,
		}

		result, err := edaxManager.DoJob(job)
		if err != nil {
			slog.Error("Failed to compute position", "error", err, "position", pos)
			continue
		}

		evaluations = append(evaluations, result.Evaluation)

		if len(evaluations) == batchSize {
			if err = client.SaveLearnedEvaluations(evaluations); err != nil {
				slog.Error("Failed to save evaluations", "error", err)
				return err
			}

			evaluations = make([]models.Evaluation, 0)
		}
	}

	// Save any remaining evaluations
	if err := client.SaveLearnedEvaluations(evaluations); err != nil {
		slog.Error("Failed to save evaluations", "error", err)
		return err
	}

	return nil
}
