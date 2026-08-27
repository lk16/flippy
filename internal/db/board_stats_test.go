package db

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// verifyBoardStatsScriptPath is the operator script that checks board_stats against boards.
const verifyBoardStatsScriptPath = "../../scripts/verify_board_stats.sql"

// TestVerifyBoardStatsScript checks the script accepts a book the triggers kept in step and rejects
// one whose counts were tampered with, so a silent pass can't hide a broken trigger.
func TestVerifyBoardStatsScript(t *testing.T) {
	repo, tx := testRepositoryWithTx(t)
	ctx := context.Background()

	script, err := os.ReadFile(verifyBoardStatsScriptPath)
	require.NoError(t, err)

	positions := testDistinctPositions(t, 12, 3)
	require.NoError(t, repo.AddPositions(ctx, positions))
	require.NoError(t, repo.SaveEvaluation(ctx, positions[0], Evaluation{Level: 20, Score: 4}))

	_, err = tx.Exec(ctx, string(script))
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `UPDATE board_stats SET count = count + 1 WHERE level = 0`)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, string(script))
	require.ErrorContains(t, err, "board_stats disagrees with boards")
}

// TestVerifyBoardStatsScript_AcceptsAnEmptiedCell checks the zero rows the triggers leave behind
// are not reported as disagreements.
func TestVerifyBoardStatsScript_AcceptsAnEmptiedCell(t *testing.T) {
	repo, tx := testRepositoryWithTx(t)
	ctx := context.Background()

	script, err := os.ReadFile(verifyBoardStatsScriptPath)
	require.NoError(t, err)

	position := testPosition(t, 12)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	_, err = tx.Exec(ctx, `DELETE FROM boards`)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, string(script))
	require.NoError(t, err)
}
