// Package loader adds boards to the DB, from the precomputed 12-disc set or extracted from game files.
package loader

import (
	"context"
	"fmt"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// SeedBoards adds every precomputed 12-disc board to the DB; idempotent, since AddBoards leaves
// existing rows untouched.
func SeedBoards(ctx context.Context, repo *db.Repository) error {
	if err := repo.AddBoards(ctx, othello.PrecomputedBoards12()); err != nil {
		return fmt.Errorf("failed to seed precomputed boards: %w", err)
	}
	return nil
}
