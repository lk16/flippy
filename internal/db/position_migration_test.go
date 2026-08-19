package db

import (
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

const (
	// positionMigrationPath is the migration that shrinks positions.position to 16 bytes.
	positionMigrationPath = "../../migrations/000003_drop_turn_from_position.up.sql"

	// positionMergeScriptPath is the operator script that proves that migration loses nothing.
	positionMergeScriptPath = "../../scripts/verify_position_merge.sql"
)

// legacyPosition encodes position the way positions.position did before migration 000003: 8 bytes of
// black discs, 8 of white discs, and a turn byte. blackToMove picks which side owns which discs,
// which is exactly the distinction the migration drops.
func legacyPosition(position othello.Position, blackToMove bool) []byte {
	buf := make([]byte, 17)
	if blackToMove {
		binary.BigEndian.PutUint64(buf[0:8], position.Player())
		binary.BigEndian.PutUint64(buf[8:16], position.Opponent())
	} else {
		binary.BigEndian.PutUint64(buf[0:8], position.Opponent())
		binary.BigEndian.PutUint64(buf[8:16], position.Player())
		buf[16] = 1
	}
	return buf
}

// restoreLegacyPositions replaces the migrated boards table with the pre-000003 one, so the tests
// below can run the real migration over real legacy rows. Safe because testRepository hands out a
// transaction that is always rolled back.
func restoreLegacyPositions(t *testing.T, repo *Repository) {
	t.Helper()

	_, err := repo.db.Exec(context.Background(), `
		DROP TABLE boards;
		CREATE TABLE boards (
		    position bytea NOT NULL PRIMARY KEY,
		    disc_count smallint NOT NULL,
		    level smallint NOT NULL DEFAULT 0,
		    score smallint NOT NULL DEFAULT 0
		);`)
	require.NoError(t, err)
}

// insertLegacyPosition adds one pre-000003 row.
func insertLegacyPosition(t *testing.T, repo *Repository, position othello.Position, blackToMove bool, level, score int) {
	t.Helper()

	_, err := repo.db.Exec(context.Background(),
		`INSERT INTO boards (position, disc_count, level, score) VALUES ($1, $2, $3, $4)`,
		legacyPosition(position, blackToMove), position.CountDiscs(), level, score)
	require.NoError(t, err)
}

// runPositionMigration applies migrations/000003 to the current transaction.
func runPositionMigration(t *testing.T, repo *Repository) {
	t.Helper()

	migration, err := os.ReadFile(positionMigrationPath)
	require.NoError(t, err)

	_, err = repo.db.Exec(context.Background(), string(migration))
	require.NoError(t, err)
}

// TestPositionMigration_ConvertsBothTurns checks the migration's byte shuffle against
// othello.Position.Bytes: a black-to-move row keeps its halves, a white-to-move one swaps them, and
// both end up as the position the Go code now writes.
func TestPositionMigration_ConvertsBothTurns(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	restoreLegacyPositions(t, repo)

	blackToMove := testPosition(t, 12).Position()
	whiteToMove := testPosition(t, 13).Position()
	insertLegacyPosition(t, repo, blackToMove, true, 20, 4)
	insertLegacyPosition(t, repo, whiteToMove, false, 22, -6)

	runPositionMigration(t, repo)

	for _, position := range []othello.Position{blackToMove, whiteToMove} {
		var discCount int
		err := repo.db.QueryRow(ctx, `SELECT disc_count FROM boards WHERE position = $1`, position.Bytes()).Scan(&discCount)
		require.NoError(t, err, "position %s should be stored under its 16-byte encoding", position)
		require.Equal(t, position.CountDiscs(), discCount)
	}

	var length int
	require.NoError(t, repo.db.QueryRow(ctx, `SELECT max(octet_length(position)) FROM boards`).Scan(&length))
	require.Equal(t, othello.PositionBytesLength, length)
}

// TestPositionMigration_MergesColorSwappedRows checks that the two rows that only differed in whose
// turn it was become one row holding the deeper of the two searches.
func TestPositionMigration_MergesColorSwappedRows(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	restoreLegacyPositions(t, repo)

	position := testPosition(t, 12).Position()
	insertLegacyPosition(t, repo, position, true, 20, 4)
	insertLegacyPosition(t, repo, position, false, 24, 4)

	runPositionMigration(t, repo)

	var count int
	require.NoError(t, repo.db.QueryRow(ctx, `SELECT count(*) FROM boards`).Scan(&count))
	require.Equal(t, 1, count)

	eval, err := repo.GetPosition(ctx, position)
	require.NoError(t, err)
	require.Equal(t, Evaluation{Level: 24, Score: 4}, eval)
}

// TestVerifyPositionMergeScript runs the operator script over a book holding a color-swapped pair
// that disagrees, which is the one case where the migration would drop a real evaluation.
func TestVerifyPositionMergeScript(t *testing.T) {
	script, err := os.ReadFile(positionMergeScriptPath)
	require.NoError(t, err)

	t.Run("passes on a consistent book", func(t *testing.T) {
		repo := testRepository(t)
		restoreLegacyPositions(t, repo)

		position := testPosition(t, 12).Position()
		insertLegacyPosition(t, repo, position, true, 20, 4)
		insertLegacyPosition(t, repo, position, false, 24, -2)

		_, err := repo.db.Exec(context.Background(), string(script))
		require.NoError(t, err)
	})

	t.Run("fails when a pair disagrees at the same level", func(t *testing.T) {
		repo := testRepository(t)
		restoreLegacyPositions(t, repo)

		position := testPosition(t, 12).Position()
		insertLegacyPosition(t, repo, position, true, 20, 4)
		insertLegacyPosition(t, repo, position, false, 20, -2)

		_, err := repo.db.Exec(context.Background(), string(script))
		require.ErrorContains(t, err, "disagree on score at the same level")
	})

	t.Run("is a no-op once the positions are 16 bytes", func(t *testing.T) {
		repo := testRepository(t)

		_, err := repo.db.Exec(context.Background(), string(script))
		require.NoError(t, err)
	})
}
