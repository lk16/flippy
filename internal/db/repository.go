package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/lk16/flippy/internal/othello"
)

// querier is the subset of *pgxpool.Pool and pgx.Tx that Repository needs,
// so it can run against either a pool or a transaction (the latter used by
// tests for per-test isolation).
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Evaluation is the current edax evaluation state stored for a Board. A
// zero-valued Evaluation represents a board that hasn't been learned yet.
type Evaluation struct {
	Level      int
	Depth      int
	Confidence int
	Score      int
}

// ErrBoardNotFound is returned when a board has no row in the boards table.
var ErrBoardNotFound = errors.New("board not found")

// Repository provides access to the boards table.
type Repository struct {
	db querier
}

// NewRepository returns a Repository backed by db, which may be a
// *pgxpool.Pool or a pgx.Tx.
func NewRepository(db querier) *Repository {
	return &Repository{db: db}
}

// AddBoards inserts boards that don't already have a row, each starting with
// a zeroed (unlearned) evaluation. Boards that already exist are left
// untouched: adding boards never updates or removes existing rows.
//
// All boards are sent as two array parameters (via UNNEST) rather than one
// placeholder pair per board, so this stays well under Postgres's parameter
// limit even when called with the full ~67k-board precomputed set.
func (r *Repository) AddBoards(ctx context.Context, boards []othello.NormalizedBoard) error {
	if len(boards) == 0 {
		return nil
	}

	positions := make([][]byte, len(boards))
	discCounts := make([]int16, len(boards))
	for i, board := range boards {
		positions[i] = board.Board().Bytes()
		discCounts[i] = int16(board.CountDiscs())
	}

	_, err := r.db.Exec(ctx,
		`INSERT INTO boards (position, disc_count)
		 SELECT * FROM UNNEST($1::bytea[], $2::smallint[])
		 ON CONFLICT (position) DO NOTHING`,
		positions, discCounts,
	)
	if err != nil {
		return fmt.Errorf("failed to add boards: %w", err)
	}

	return nil
}

// GetBoard returns the stored evaluation for board's normalized form, or
// ErrBoardNotFound if it has no row.
func (r *Repository) GetBoard(ctx context.Context, board othello.Board) (Evaluation, error) {
	normalized := board.Normalize()

	var eval Evaluation
	err := r.db.QueryRow(ctx,
		`SELECT level, depth, confidence, score FROM boards WHERE position = $1`,
		normalized.Board().Bytes(),
	).Scan(&eval.Level, &eval.Depth, &eval.Confidence, &eval.Score)

	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, ErrBoardNotFound
	}
	if err != nil {
		return Evaluation{}, fmt.Errorf("failed to get board: %w", err)
	}

	return eval, nil
}

// SaveEvaluation updates an existing board's evaluation, but only if
// (eval.Level, eval.Confidence) is lexicographically greater than the
// stored (level, confidence) — i.e. a higher level, or an equal level with
// higher confidence. Otherwise it's a silent no-op: not every learn result
// improves on what's already stored, and that's not an error.
//
// It returns ErrBoardNotFound only if the board has no row at all: it never
// inserts a new row, since adding boards and learning are separate
// operations that must never implicitly perform each other's job.
func (r *Repository) SaveEvaluation(ctx context.Context, board othello.NormalizedBoard, eval Evaluation) error {
	position := board.Board().Bytes()

	// The UPDATE's own WHERE clause tells us whether it improved the row,
	// but not whether the row exists at all when it didn't fire. The outer
	// EXISTS, run in the same query, answers that in one round trip.
	var exists bool
	err := r.db.QueryRow(ctx,
		`WITH updated AS (
			UPDATE boards
			SET level = $1, depth = $2, confidence = $3, score = $4
			WHERE position = $5 AND ($1::smallint, $3::smallint) > (level, confidence)
			RETURNING position
		 )
		 SELECT EXISTS (SELECT 1 FROM boards WHERE position = $5)`,
		eval.Level, eval.Depth, eval.Confidence, eval.Score, position,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to save evaluation: %w", err)
	}

	if !exists {
		return ErrBoardNotFound
	}

	return nil
}

// BoardEvaluation pairs a NormalizedBoard with its current evaluation, for
// bulk queries that need both.
type BoardEvaluation struct {
	Board      othello.NormalizedBoard
	Evaluation Evaluation
}

// ListLearnable returns up to limit boards with disc counts in
// [minDiscs, maxDiscs], ordered by disc count then level (both ascending) —
// the order in which job assignment should offer them out.
func (r *Repository) ListLearnable(ctx context.Context, minDiscs, maxDiscs, limit int) ([]BoardEvaluation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT position, level, depth, confidence, score
		 FROM boards
		 WHERE disc_count BETWEEN $1 AND $2
		 ORDER BY disc_count, level
		 LIMIT $3`,
		minDiscs, maxDiscs, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list learnable boards: %w", err)
	}
	defer rows.Close()

	var results []BoardEvaluation
	for rows.Next() {
		var position []byte
		var eval Evaluation
		if err := rows.Scan(&position, &eval.Level, &eval.Depth, &eval.Confidence, &eval.Score); err != nil {
			return nil, fmt.Errorf("failed to scan learnable board: %w", err)
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
		return nil, fmt.Errorf("failed to list learnable boards: %w", err)
	}

	return results, nil
}

// LevelStat is the count of boards at a given (disc count, level) pair.
type LevelStat struct {
	DiscCount int
	Level     int
	Count     int
}

// Stats returns, for every (disc count, level) pair that has at least one
// board, how many boards are at that pair. Pairs with no boards are omitted
// rather than returned with a zero count.
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
