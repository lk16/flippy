package db

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/edax"
)

// verifyScriptPath is the operator script that proves migration 000002's column drop is lossless.
const verifyScriptPath = "../../scripts/verify_derived_columns.sql"

// TestVerifyDerivedColumnsScript checks the script's SQL transcription of edax's level table
// against edax.SearchParams over every (disc count, level) pair, so the two copies can't drift.
func TestVerifyDerivedColumnsScript(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	script, err := os.ReadFile(verifyScriptPath)
	require.NoError(t, err)

	// The script's own report is a no-op here: the test DB is migrated, so the columns are gone.
	_, err = repo.db.Exec(ctx, string(script))
	require.NoError(t, err)

	rows, err := repo.db.Query(ctx,
		`SELECT disc_count, level, (edax_search_params(disc_count, level)).*
		 FROM generate_series(4, 64) AS disc_count, generate_series(0, $1::int) AS level`,
		edax.MaxLevel,
	)
	require.NoError(t, err)
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var discCount, level, depth, confidence int
		require.NoError(t, rows.Scan(&discCount, &level, &depth, &confidence))

		wantDepth, wantConfidence := edax.SearchParams(discCount, level)
		require.Equal(t, wantDepth, depth, "depth for disc_count=%d level=%d", discCount, level)
		require.Equal(t, wantConfidence, confidence, "confidence for disc_count=%d level=%d", discCount, level)
		checked++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, 61*(edax.MaxLevel+1), checked)
}
