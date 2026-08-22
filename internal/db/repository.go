package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/lk16/flippy/internal/othello"
)

// querier is the subset of *pgxpool.Pool and pgx.Tx that Repository needs, so it can run against either.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Evaluation is the current edax evaluation state stored for a position; the zero value means unlearned.
// Depth and confidence are not stored: they follow from (disc count, level) via edax.SearchParams.
type Evaluation struct {
	Level int
	Score int
}

// IsLearned reports whether e is an actual edax result rather than the not-yet-learned zero value.
func (e Evaluation) IsLearned() bool {
	return e.Level > 0
}

// ErrPositionNotFound is returned when a position has no row in the boards table.
var ErrPositionNotFound = errors.New("position not found")

// Repository provides access to the boards table.
type Repository struct {
	db querier
}

// NewRepository returns a Repository backed by db, which may be a *pgxpool.Pool or a pgx.Tx.
func NewRepository(db querier) *Repository {
	return &Repository{db: db}
}

// Ping verifies the database is reachable with a trivial query.
func (r *Repository) Ping(ctx context.Context) error {
	var one int
	if err := r.db.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	return nil
}

// AddPositions inserts positions that don't already have a row, leaving existing rows untouched.
// Use AddPositionsInserted instead when the number of rows actually inserted is needed.
func (r *Repository) AddPositions(ctx context.Context, positions []othello.NormalizedPosition) error {
	_, err := r.AddPositionsInserted(ctx, positions)
	return err
}

// AddPositionsInserted inserts positions that don't already have a row and returns the number
// actually inserted (existing ones are skipped, not counted). Rows are sent via UNNEST rather than
// one placeholder pair each, to stay under Postgres's parameter limit.
func (r *Repository) AddPositionsInserted(ctx context.Context, positions []othello.NormalizedPosition) (int, error) {
	if len(positions) == 0 {
		return 0, nil
	}

	encoded := make([][]byte, len(positions))
	discCounts := make([]int16, len(positions))
	for i, position := range positions {
		encoded[i] = position.Position().Bytes()
		discCounts[i] = int16(position.CountDiscs())
	}

	tag, err := r.db.Exec(ctx,
		`INSERT INTO boards (position, disc_count)
		 SELECT * FROM UNNEST($1::bytea[], $2::smallint[])
		 ON CONFLICT (position) DO NOTHING`,
		encoded, discCounts,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to add positions: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// GetPosition returns the stored evaluation for position's normalized form, or ErrPositionNotFound.
func (r *Repository) GetPosition(ctx context.Context, position othello.Position) (Evaluation, error) {
	normalized := position.Normalize()

	var eval Evaluation
	err := r.db.QueryRow(ctx,
		`SELECT level, score FROM boards WHERE position = $1`,
		normalized.Position().Bytes(),
	).Scan(&eval.Level, &eval.Score)

	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, ErrPositionNotFound
	}
	if err != nil {
		return Evaluation{}, fmt.Errorf("failed to get position: %w", err)
	}

	return eval, nil
}

// SaveEvaluation updates an existing position's evaluation, but only if its level improves on
// what's stored; a non-improving result is a silent no-op. Never inserts a row:
// ErrPositionNotFound if none exists.
func (r *Repository) SaveEvaluation(ctx context.Context, position othello.NormalizedPosition, eval Evaluation) error {
	encoded := position.Position().Bytes()

	// The outer EXISTS tells whether the row exists at all when the UPDATE's WHERE doesn't fire.
	var exists bool
	err := r.db.QueryRow(ctx,
		`WITH updated AS (
			UPDATE boards
			SET level = $1, score = $2
			WHERE position = $3 AND $1::smallint > level
			RETURNING position
		 )
		 SELECT EXISTS (SELECT 1 FROM boards WHERE position = $3)`,
		eval.Level, eval.Score, encoded,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to save evaluation: %w", err)
	}

	if !exists {
		return ErrPositionNotFound
	}

	return nil
}

// PositionEvaluation pairs a NormalizedPosition with its current evaluation.
type PositionEvaluation struct {
	Position   othello.NormalizedPosition
	Evaluation Evaluation
}

// ListLearnable returns up to limit positions in [minDiscs, maxDiscs] below their target level
// (leafLevel for leafDiscs, deeperLevel beyond); filtering in SQL keeps learned leafDiscs rows from
// starving the rest. minDiscs may be raised above leafDiscs without changing which count is leaf.
func (r *Repository) ListLearnable(ctx context.Context, minDiscs, maxDiscs, leafDiscs, leafLevel, deeperLevel, limit int) ([]PositionEvaluation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, level, score
		 FROM boards
		 WHERE disc_count BETWEEN $1 AND $2
		   AND level < CASE WHEN disc_count = $3 THEN $4::smallint ELSE $5::smallint END
		 ORDER BY disc_count, level
		 LIMIT $6`,
		minDiscs, maxDiscs, leafDiscs, leafLevel, deeperLevel, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list learnable positions: %w", err)
	}
	defer rows.Close()

	results, err := scanPositionEvaluations(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list learnable positions: %w", err)
	}

	return results, nil
}

// EvaluatedPositions returns every learned (level > 0) position with exactly discCount discs.
func (r *Repository) EvaluatedPositions(ctx context.Context, discCount int) ([]PositionEvaluation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, level, score
		 FROM boards
		 WHERE disc_count = $1 AND level > 0`,
		discCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list evaluated positions: %w", err)
	}
	defer rows.Close()

	results, err := scanPositionEvaluations(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list evaluated positions: %w", err)
	}

	return results, nil
}

// scanPositionEvaluations scans rows of (position, level, score) into PositionEvaluations.
func scanPositionEvaluations(rows pgx.Rows) ([]PositionEvaluation, error) {
	var results []PositionEvaluation
	for rows.Next() {
		var encoded []byte
		var eval Evaluation
		if err := rows.Scan(&encoded, &eval.Level, &eval.Score); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		position, err := othello.ParsePositionBytes(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stored position: %w", err)
		}

		normalized, err := othello.NewNormalizedPosition(position)
		if err != nil {
			return nil, fmt.Errorf("stored position is not normalized: %w", err)
		}

		results = append(results, PositionEvaluation{Position: normalized, Evaluation: eval})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	return results, nil
}

// LevelStat is the count of positions at a given (disc count, level) pair.
type LevelStat struct {
	DiscCount int
	Level     int
	Count     int
}

// Stats returns position counts per (disc count, level) pair, omitting empty pairs.
func (r *Repository) Stats(ctx context.Context) ([]LevelStat, error) {
	rows, err := r.db.Query(ctx,
		`SELECT disc_count, level, count(*)
		 FROM boards
		 GROUP BY disc_count, level
		 ORDER BY disc_count, level`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}
	defer rows.Close()

	var stats []LevelStat
	for rows.Next() {
		var stat LevelStat
		if err := rows.Scan(&stat.DiscCount, &stat.Level, &stat.Count); err != nil {
			return nil, fmt.Errorf("failed to scan stat: %w", err)
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return stats, nil
}
