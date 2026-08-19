// Package loader adds positions to the DB, from the precomputed 12-disc set or extracted from game files.
package loader

import (
	"context"
	"fmt"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// SeedPositions adds every precomputed 12-disc position to the DB; idempotent, since AddPositions
// leaves existing rows untouched.
func SeedPositions(ctx context.Context, repo *db.Repository) error {
	if err := repo.AddPositions(ctx, othello.PrecomputedPositions12()); err != nil {
		return fmt.Errorf("failed to seed precomputed positions: %w", err)
	}
	return nil
}
