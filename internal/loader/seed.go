// Package loader adds boards to the DB: today, just the embedded
// precomputed 12-disc set; later, extraction from imported game files
// (wtb/pgn/move-string).
package loader

import (
	"context"
	"fmt"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// SeedBoards adds every precomputed 12-disc board to the DB, so there's
// something for workers to learn and something to show on the stats and
// clients pages. It's idempotent, per AddBoards' own semantics: boards
// that already have a row (including any already-learned evaluation) are
// left untouched.
func SeedBoards(ctx context.Context, repo *db.Repository) error {
	if err := repo.AddBoards(ctx, othello.PrecomputedBoards12()); err != nil {
		return fmt.Errorf("failed to seed precomputed boards: %w", err)
	}
	return nil
}
