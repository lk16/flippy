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

// Evaluation is the current edax evaluation state stored for a Board; the zero value means unlearned.
// Depth and confidence are not stored: they follow from (disc count, level) via edax.SearchParams.
type Evaluation struct {
	Level int
	Score int
}

// IsLearned reports whether e is an actual edax result rather than the not-yet-learned zero value.
func (e Evaluation) IsLearned() bool {
	return e.Level > 0
}

// ErrBoardNotFound is returned when a board has no row in the boards table.
var ErrBoardNotFound = errors.New("board not found")

// Repository provides access to the boards table.
type Repository struct {
	db querier
}

// NewRepository returns a Repository backed by db, which may be a *pgxpool.Pool or a pgx.Tx.
func NewRepository(db querier) *Repository {
	return &Repository{db: db}
}

// AddBoards inserts boards that don't already have a row, leaving existing rows untouched. Use
// AddBoardsInserted instead when the number of rows actually inserted is needed.
func (r *Repository) AddBoards(ctx context.Context, boards []othello.NormalizedBoard) error {
	_, err := r.AddBoardsInserted(ctx, boards)
	return err
}

// AddBoardsInserted inserts boards that don't already have a row and returns the number actually
// inserted (existing boards are skipped, not counted). Boards are sent via UNNEST rather than one
// placeholder pair each, to stay under Postgres's parameter limit.
func (r *Repository) AddBoardsInserted(ctx context.Context, boards []othello.NormalizedBoard) (int, error) {
	if len(boards) == 0 {
		return 0, nil
	}

	positions := make([][]byte, len(boards))
	discCounts := make([]int16, len(boards))
	for i, board := range boards {
		positions[i] = board.Board().Bytes()
		discCounts[i] = int16(board.CountDiscs())
	}

	tag, err := r.db.Exec(ctx,
		`INSERT INTO boards (position, disc_count)
		 SELECT * FROM UNNEST($1::bytea[], $2::smallint[])
		 ON CONFLICT (position) DO NOTHING`,
		positions, discCounts,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to add boards: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

// GetBoard returns the stored evaluation for board's normalized form, or ErrBoardNotFound.
func (r *Repository) GetBoard(ctx context.Context, board othello.Board) (Evaluation, error) {
	normalized := board.Normalize()

	var eval Evaluation
	err := r.db.QueryRow(ctx,
		`SELECT level, score FROM boards WHERE position = $1`,
		normalized.Board().Bytes(),
	).Scan(&eval.Level, &eval.Score)

	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, ErrBoardNotFound
	}
	if err != nil {
		return Evaluation{}, fmt.Errorf("failed to get board: %w", err)
	}

	return eval, nil
}

// SaveEvaluation updates an existing board's evaluation, but only if its level improves on what's
// stored; a non-improving result is a silent no-op. Never inserts a row: ErrBoardNotFound if none exists.
func (r *Repository) SaveEvaluation(ctx context.Context, board othello.NormalizedBoard, eval Evaluation) error {
	position := board.Board().Bytes()

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
		eval.Level, eval.Score, position,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to save evaluation: %w", err)
	}

	if !exists {
		return ErrBoardNotFound
	}

	return nil
}

// BoardEvaluation pairs a NormalizedBoard with its current evaluation.
type BoardEvaluation struct {
	Board      othello.NormalizedBoard
	Evaluation Evaluation
}

// ListLearnable returns up to limit boards in [minDiscs, maxDiscs] below their target level
// (leafLevel for leafDiscs, deeperLevel beyond); filtering in SQL keeps learned leafDiscs rows from
// starving the rest. minDiscs may be raised above leafDiscs without changing which count is leaf.
func (r *Repository) ListLearnable(ctx context.Context, minDiscs, maxDiscs, leafDiscs, leafLevel, deeperLevel, limit int) ([]BoardEvaluation, error) {
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
		return nil, fmt.Errorf("failed to list learnable boards: %w", err)
	}
	defer rows.Close()

	results, err := scanBoardEvaluations(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list learnable boards: %w", err)
	}

	return results, nil
}

// EvaluatedBoards returns every learned (level > 0) board with exactly discCount discs.
func (r *Repository) EvaluatedBoards(ctx context.Context, discCount int) ([]BoardEvaluation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, level, score
		 FROM boards
		 WHERE disc_count = $1 AND level > 0`,
		discCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list evaluated boards: %w", err)
	}
	defer rows.Close()

	results, err := scanBoardEvaluations(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list evaluated boards: %w", err)
	}

	return results, nil
}

// scanBoardEvaluations scans rows of (position, level, score) into BoardEvaluations.
func scanBoardEvaluations(rows pgx.Rows) ([]BoardEvaluation, error) {
	var results []BoardEvaluation
	for rows.Next() {
		var position []byte
		var eval Evaluation
		if err := rows.Scan(&position, &eval.Level, &eval.Score); err != nil {
			return nil, fmt.Errorf("failed to scan board: %w", err)
		}

		board, err := othello.ParseBoardBytes(position)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stored board: %w", err)
		}

		normalized, err := othello.NewNormalizedBoard(board)
		if err != nil {
			return nil, fmt.Errorf("stored board is not normalized: %w", err)
		}

		results = append(results, BoardEvaluation{Board: normalized, Evaluation: eval})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read board rows: %w", err)
	}

	return results, nil
}

// LevelStat is the count of boards at a given (disc count, level) pair.
type LevelStat struct {
	DiscCount int
	Level     int
	Count     int
}

// Stats returns board counts per (disc count, level) pair, omitting empty pairs.
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
