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

// MinSavableDiscs and MaxSavableDiscs bound the disc counts the boards table holds a row for.
// Below the minimum every position is minimax-derived from the rows at it (see internal/book, whose
// LeafDiscs is this constant); above the maximum a position is not worth a search. The range lives
// here rather than in internal/book so AddPositionsInserted can enforce it -- validation is the
// app's job by design, the table has no CHECK constraint.
const (
	MinSavableDiscs = 12
	MaxSavableDiscs = 30
)

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
// actually inserted (existing ones are skipped, not counted). Positions outside
// [MinSavableDiscs, MaxSavableDiscs] are dropped here rather than at the call sites, so no caller
// can put a row in the table that nothing will ever learn. Rows are sent via UNNEST rather than one
// placeholder pair each, to stay under Postgres's parameter limit.
func (r *Repository) AddPositionsInserted(ctx context.Context, positions []othello.NormalizedPosition) (int, error) {
	encoded := make([][]byte, 0, len(positions))
	discCounts := make([]int16, 0, len(positions))
	for _, position := range positions {
		discCount := position.CountDiscs()
		if discCount < MinSavableDiscs || discCount > MaxSavableDiscs {
			continue
		}
		encoded = append(encoded, position.Position().Bytes())
		discCounts = append(discCounts, int16(discCount))
	}

	if len(encoded) == 0 {
		return 0, nil
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

// SaveOutcome reports what SaveEvaluationOutcome did: whether the row was updated, and the level
// it held beforehand (0 for unlearned).
type SaveOutcome struct {
	Updated  bool
	OldLevel int
}

// SaveEvaluation updates an existing position's evaluation, but only if its level improves on
// what's stored; a non-improving result is a silent no-op. Never inserts a row:
// ErrPositionNotFound if none exists. Use SaveEvaluationOutcome instead when the effect matters.
func (r *Repository) SaveEvaluation(ctx context.Context, position othello.NormalizedPosition, eval Evaluation) error {
	_, err := r.SaveEvaluationOutcome(ctx, position, eval)
	return err
}

// SaveEvaluationOutcome is SaveEvaluation, also reporting whether the row was updated and the
// level it held before, so callers can maintain per-level counters incrementally.
func (r *Repository) SaveEvaluationOutcome(
	ctx context.Context, position othello.NormalizedPosition, eval Evaluation,
) (SaveOutcome, error) {
	encoded := position.Position().Bytes()

	// The old CTE reads the statement's snapshot, i.e. the pre-update level; zero rows from it
	// means the position has no row at all.
	var outcome SaveOutcome
	err := r.db.QueryRow(ctx,
		`WITH old AS (
			SELECT level FROM boards WHERE position = $3
		 ), updated AS (
			UPDATE boards
			SET level = $1, score = $2
			WHERE position = $3 AND $1::smallint > level
			RETURNING position
		 )
		 SELECT old.level, EXISTS (SELECT 1 FROM updated) FROM old`,
		eval.Level, eval.Score, encoded,
	).Scan(&outcome.OldLevel, &outcome.Updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return SaveOutcome{}, ErrPositionNotFound
	}
	if err != nil {
		return SaveOutcome{}, fmt.Errorf("failed to save evaluation: %w", err)
	}

	return outcome, nil
}

// PositionEvaluation pairs a NormalizedPosition with its current evaluation.
type PositionEvaluation struct {
	Position   othello.NormalizedPosition
	Evaluation Evaluation
}

// LearnableCursor is a point in ListPartiallyLearned's ordering. The zero value sorts before every
// row, so it starts a scan at the beginning.
type LearnableCursor struct {
	DiscCount int
	Level     int
	Position  othello.NormalizedPosition
}

// Cursor returns the cursor pointing at pe, for resuming a scan after it.
func (pe PositionEvaluation) Cursor() LearnableCursor {
	return LearnableCursor{
		DiscCount: pe.Position.CountDiscs(),
		Level:     pe.Evaluation.Level,
		Position:  pe.Position,
	}
}

// UnlearnedQuery bounds a ListUnlearned scan: never-searched positions in [MinDiscs, MaxDiscs],
// at most Limit of them.
type UnlearnedQuery struct {
	MinDiscs int
	MaxDiscs int
	Limit    int
}

// ListUnlearned returns up to q.Limit never-searched positions, by disc count then position; their
// Evaluation is the zero value. There is no cursor: a row leaves the set as soon as it is searched,
// so starting every scan at the beginning is what lets a row added later be picked up right away.
func (r *Repository) ListUnlearned(ctx context.Context, q UnlearnedQuery) ([]PositionEvaluation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, level, score
		 FROM boards
		 WHERE level = 0 AND disc_count BETWEEN $1 AND $2
		 ORDER BY disc_count, position
		 LIMIT $3`,
		q.MinDiscs, q.MaxDiscs, q.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list unlearned positions: %w", err)
	}
	defer rows.Close()

	results, err := scanPositionEvaluations(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list unlearned positions: %w", err)
	}

	return results, nil
}

// LearnableQuery bounds a ListPartiallyLearned scan: positions in [MinDiscs, MaxDiscs] below their
// target level, which is LeafLevel at LeafDiscs and DeeperLevel above it. MinDiscs may be raised
// above LeafDiscs without changing which count is leaf. After resumes a scan, and Limit caps the
// rows.
type LearnableQuery struct {
	MinDiscs    int
	MaxDiscs    int
	LeafDiscs   int
	LeafLevel   int
	DeeperLevel int
	After       LearnableCursor
	Limit       int
}

// ListPartiallyLearned returns up to q.Limit searched-but-below-target positions matching q, by
// disc count, then level, then position. Ordering by level orders by the search it stands for:
// (depth, confidence) never decreases as level rises within a disc count (see
// TestSearchParamsRiseWithLevel). Filtering in SQL keeps learned LeafDiscs rows from starving the
// rest.
func (r *Repository) ListPartiallyLearned(ctx context.Context, q LearnableQuery) ([]PositionEvaluation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, level, score
		 FROM boards
		 WHERE disc_count BETWEEN $1 AND $2
		   AND level > 0
		   AND level < CASE WHEN disc_count = $3 THEN $4::smallint ELSE $5::smallint END
		   AND (disc_count, level, position) > ($6::smallint, $7::smallint, $8::bytea)
		 ORDER BY disc_count, level, position
		 LIMIT $9`,
		q.MinDiscs, q.MaxDiscs, q.LeafDiscs, q.LeafLevel, q.DeeperLevel,
		q.After.DiscCount, q.After.Level, q.After.Position.Position().Bytes(),
		q.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list partially learned positions: %w", err)
	}
	defer rows.Close()

	results, err := scanPositionEvaluations(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list partially learned positions: %w", err)
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
