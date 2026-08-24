package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testRepository returns a Repository backed by a transaction rolled back when the test ends;
// skips the test if FLIPPY_POSTGRES_URL isn't set.
func testRepository(t *testing.T) *Repository {
	t.Helper()

	url := os.Getenv("FLIPPY_POSTGRES_URL")
	if url == "" {
		t.Skip("FLIPPY_POSTGRES_URL not set; skipping test requiring Postgres")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tx.Rollback(ctx)
	})

	return NewRepository(tx)
}

var testPosition = othellotest.Position

// testUnnormalizedPosition returns a Position with exactly discs discs that is not its own
// normalized form, so a caller can check that a lookup normalizes what it is given.
func testUnnormalizedPosition(t *testing.T, discs int) othello.Position {
	t.Helper()

	for _, parent := range othellotest.DistinctPositions(t, discs-1, 20) {
		for _, child := range parent.Position().Children() {
			if child.CountDiscs() == discs && !child.IsNormalized() {
				return child
			}
		}
	}

	t.Fatalf("no unnormalized position with %d discs found", discs)
	return othello.Position{}
}

var testDistinctPositions = othellotest.DistinctPositions

func TestEvaluation_IsLearned(t *testing.T) {
	require.False(t, Evaluation{}.IsLearned())
	require.False(t, Evaluation{Score: 2}.IsLearned())
	require.True(t, Evaluation{Level: 16, Score: 3}.IsLearned())
}

func TestRepository_AddPositions_GetBoard(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	position := testPosition(t, 12)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	eval, err := repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, Evaluation{}, eval)
}

func TestRepository_AddPositions_DoesNotOverwriteExisting(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	position := testPosition(t, 12)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	saved := Evaluation{Level: 20, Score: 4}
	require.NoError(t, repo.SaveEvaluation(ctx, position, saved))

	// Adding the same position again must not clobber the evaluation just saved.
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	eval, err := repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, saved, eval)
}

func TestRepository_AddPositions_Empty(t *testing.T) {
	repo := testRepository(t)
	require.NoError(t, repo.AddPositions(context.Background(), nil))
}

func TestRepository_AddPositionsInserted_CountsOnlyNewRows(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:5]

	inserted, err := repo.AddPositionsInserted(ctx, positions)
	require.NoError(t, err)
	require.Equal(t, len(positions), inserted)

	// Re-inserting the same positions must count zero new rows (ON CONFLICT DO
	// NOTHING skips them), rather than reporting the batch size again.
	inserted, err = repo.AddPositionsInserted(ctx, positions)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)

	// A batch that overlaps existing rows counts only the genuinely new ones.
	extra := othello.PrecomputedPositions12()[3:8] // 2 overlap, 3 new
	inserted, err = repo.AddPositionsInserted(ctx, extra)
	require.NoError(t, err)
	require.Equal(t, 3, inserted)
}

func TestRepository_AddPositionsInserted_Empty(t *testing.T) {
	repo := testRepository(t)
	inserted, err := repo.AddPositionsInserted(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
}

// TestRepository_AddPositionsInserted_RejectsOutOfRangeDiscCounts is the guard that keeps rows
// nothing will ever learn out of the table: below MinSavableDiscs internal/book derives the score
// by minimax, above MaxSavableDiscs no job is ever handed out.
func TestRepository_AddPositionsInserted_RejectsOutOfRangeDiscCounts(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	tooFew := testPosition(t, MinSavableDiscs-1)
	tooMany := testPosition(t, MaxSavableDiscs+1)
	savable := testPosition(t, MinSavableDiscs)

	inserted, err := repo.AddPositionsInserted(ctx, []othello.NormalizedPosition{tooFew, savable, tooMany})
	require.NoError(t, err)
	require.Equal(t, 1, inserted, "only the in-range position is stored")

	for _, position := range []othello.NormalizedPosition{tooFew, tooMany} {
		_, err := repo.GetPosition(ctx, position.Position())
		require.ErrorIs(t, err, ErrPositionNotFound)
	}

	_, err = repo.GetPosition(ctx, savable.Position())
	require.NoError(t, err)
}

// TestRepository_AddPositionsInserted_OnlyOutOfRange covers the batch that filters down to nothing:
// no query runs, and the caller is told no row was inserted rather than getting an error.
func TestRepository_AddPositionsInserted_OnlyOutOfRange(t *testing.T) {
	repo := testRepository(t)

	inserted, err := repo.AddPositionsInserted(context.Background(),
		[]othello.NormalizedPosition{testPosition(t, MinSavableDiscs-1)})
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
}

func TestRepository_AddPositions_Multiple(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:100]

	require.NoError(t, repo.AddPositions(ctx, positions))

	for _, position := range positions {
		eval, err := repo.GetPosition(ctx, position.Position())
		require.NoError(t, err)
		require.Equal(t, Evaluation{}, eval)
	}
}

func TestRepository_GetPosition_NotFound(t *testing.T) {
	repo := testRepository(t)
	position := testPosition(t, 12)

	_, err := repo.GetPosition(context.Background(), position.Position())
	require.ErrorIs(t, err, ErrPositionNotFound)
}

func TestRepository_GetPosition_NormalizesInput(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	// Savable, so AddPositions keeps it, but not the normalized form of itself.
	position := testUnnormalizedPosition(t, MinSavableDiscs+1)
	normalized := position.Normalize()
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{normalized}))

	eval, err := repo.GetPosition(ctx, position)
	require.NoError(t, err)
	require.Equal(t, Evaluation{}, eval)
}

func TestRepository_SaveEvaluation_NotFound(t *testing.T) {
	repo := testRepository(t)
	position := testPosition(t, 12)

	err := repo.SaveEvaluation(context.Background(), position, Evaluation{Level: 20})
	require.ErrorIs(t, err, ErrPositionNotFound)
}

func TestRepository_SaveEvaluation_Updates(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	position := testPosition(t, 12)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	want := Evaluation{Level: 24, Score: -6}
	require.NoError(t, repo.SaveEvaluation(ctx, position, want))

	got, err := repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestRepository_SaveEvaluationOutcome(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	position := testPosition(t, 12)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	outcome, err := repo.SaveEvaluationOutcome(ctx, position, Evaluation{Level: 20, Score: 4})
	require.NoError(t, err)
	require.Equal(t, SaveOutcome{Updated: true, OldLevel: 0}, outcome)

	outcome, err = repo.SaveEvaluationOutcome(ctx, position, Evaluation{Level: 24, Score: 2})
	require.NoError(t, err)
	require.Equal(t, SaveOutcome{Updated: true, OldLevel: 20}, outcome)

	// Non-improving level: row untouched, reported as such.
	outcome, err = repo.SaveEvaluationOutcome(ctx, position, Evaluation{Level: 24, Score: 6})
	require.NoError(t, err)
	require.Equal(t, SaveOutcome{Updated: false, OldLevel: 24}, outcome)

	_, err = repo.SaveEvaluationOutcome(ctx, testPosition(t, 13), Evaluation{Level: 20})
	require.ErrorIs(t, err, ErrPositionNotFound)
}

// TestRepository_SaveEvaluation_LowerLevelIsNoOp checks that a shallower search never overwrites a
// deeper one, whatever score it found.
func TestRepository_SaveEvaluation_LowerLevelIsNoOp(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	position := testPosition(t, 12)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	deep := Evaluation{Level: 24, Score: 2}
	require.NoError(t, repo.SaveEvaluation(ctx, position, deep))

	require.NoError(t, repo.SaveEvaluation(ctx, position, Evaluation{Level: 20, Score: 3}))

	got, err := repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, deep, got)
}

func TestRepository_SaveEvaluation_NoOpWhenNotBetter(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	position := testPosition(t, 12)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	first := Evaluation{Level: 20, Score: 2}
	require.NoError(t, repo.SaveEvaluation(ctx, position, first))

	// Same level: the same search, so this is a no-op, not an error -- even if the score differs
	// (parallel edax searches are not bit-for-bit deterministic).
	require.NoError(t, repo.SaveEvaluation(ctx, position, Evaluation{Level: 20, Score: 3}))

	got, err := repo.GetPosition(ctx, position.Position())
	require.NoError(t, err)
	require.Equal(t, first, got)
}

// testLearnableQuery returns the query these tests share — 12-disc leaves needing level 24, up to
// 30 discs — varying only the fields a given case is about.
func testLearnableQuery(minDiscs, deeperLevel, limit int) LearnableQuery {
	return LearnableQuery{
		MinDiscs:    minDiscs,
		MaxDiscs:    30,
		LeafDiscs:   12,
		LeafLevel:   24,
		DeeperLevel: deeperLevel,
		Limit:       limit,
	}
}

func TestRepository_ListLearnable_OrdersByDiscCountThenLevel(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position12 := testPosition(t, 12)
	position13s := testDistinctPositions(t, 13, 2)
	position13, position13Other := position13s[0], position13s[1]

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position12, position13, position13Other}))
	require.NoError(t, repo.SaveEvaluation(ctx, position13, Evaluation{Level: 10, Score: 0}))
	require.NoError(t, repo.SaveEvaluation(ctx, position13Other, Evaluation{Level: 20, Score: 0}))

	results, err := repo.ListLearnable(ctx, testLearnableQuery(12, 24, 10))
	require.NoError(t, err)
	require.Len(t, results, 3)

	require.Equal(t, position12, results[0].Position)
	require.Equal(t, position13, results[1].Position)
	require.Equal(t, position13Other, results[2].Position)
}

// TestRepository_ListLearnable_LeafLevelDoesNotStarveDeeperCandidates covers the level cutoff's
// purpose: once every minDiscs position reaches leafLevel, a batch without the filter would consist
// entirely of those (they sort first), starving deeper positions.
func TestRepository_ListLearnable_LeafLevelDoesNotStarveDeeperCandidates(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	leafBoards := testDistinctPositions(t, 12, 5)
	require.NoError(t, repo.AddPositions(ctx, leafBoards))
	for _, position := range leafBoards {
		require.NoError(t, repo.SaveEvaluation(ctx, position, Evaluation{Level: 24, Score: 0}))
	}

	position13 := testPosition(t, 13)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position13}))

	// A batch smaller than the number of fully-learned leaves: without the
	// level filter, this would return only leaves and miss position13 entirely.
	results, err := repo.ListLearnable(ctx, testLearnableQuery(12, 16, 3))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, position13, results[0].Position)
}

// TestRepository_ListLearnable_MinDiscsAboveLeafDiscsKeepsLeafThreshold covers a caller raising minDiscs
// above leafDiscs (e.g. a cached "already fully learned up to here" floor): the leaf-level threshold
// must still apply only to leafDiscs, not to whatever minDiscs happens to be.
func TestRepository_ListLearnable_MinDiscsAboveLeafDiscsKeepsLeafThreshold(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position13 := testPosition(t, 13)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position13}))
	// Above deeperLevel (16) but below leafLevel (24): distinguishes the two thresholds.
	require.NoError(t, repo.SaveEvaluation(ctx, position13, Evaluation{Level: 20, Score: 0}))

	// position13 must be judged against deeperLevel (16), not leafLevel (24): a bug binding the leaf
	// check to minDiscs instead of leafDiscs would wrongly return position13 as needing level 24.
	results, err := repo.ListLearnable(ctx, testLearnableQuery(13, 16, 10))
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestRepository_ListLearnable_FiltersByDiscCountRange(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position12 := testPosition(t, 12)
	position35 := testPosition(t, 35)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position12, position35}))

	results, err := repo.ListLearnable(ctx, testLearnableQuery(12, 24, 10))
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, position12, results[0].Position)
}

func TestRepository_ListLearnable_RespectsLimit(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:5]
	require.NoError(t, repo.AddPositions(ctx, positions))

	results, err := repo.ListLearnable(ctx, testLearnableQuery(12, 24, 3))
	require.NoError(t, err)
	require.Len(t, results, 3)
}

func TestRepository_ListLearnable_Empty(t *testing.T) {
	repo := testRepository(t)

	results, err := repo.ListLearnable(context.Background(), testLearnableQuery(12, 24, 10))
	require.NoError(t, err)
	require.Empty(t, results)
}

// TestRepository_ListLearnable_AfterResumesWhereTheLastScanStopped walks the whole learnable set one
// row at a time, which is what a cursor sweep does across refills.
func TestRepository_ListLearnable_AfterResumesWhereTheLastScanStopped(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:5]
	require.NoError(t, repo.AddPositions(ctx, positions))

	query := testLearnableQuery(12, 24, 1)
	var seen []othello.NormalizedPosition
	for range len(positions) {
		results, err := repo.ListLearnable(ctx, query)
		require.NoError(t, err)
		require.Len(t, results, 1)

		seen = append(seen, results[0].Position)
		query.After = results[0].Cursor()
	}

	require.ElementsMatch(t, positions, seen)

	// The sweep is exhausted: the caller takes this as its cue to wrap.
	results, err := repo.ListLearnable(ctx, query)
	require.NoError(t, err)
	require.Empty(t, results)
}

// TestRepository_ListLearnable_AfterOrdersRowsSharingDiscCountAndLevel covers the tiebreak a cursor
// needs: without position in the ordering, rows equal on (disc_count, level) could repeat or be
// skipped across scans.
func TestRepository_ListLearnable_AfterOrdersRowsSharingDiscCountAndLevel(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:3]
	require.NoError(t, repo.AddPositions(ctx, positions))

	all, err := repo.ListLearnable(ctx, testLearnableQuery(12, 24, 10))
	require.NoError(t, err)
	require.Len(t, all, 3)

	query := testLearnableQuery(12, 24, 10)
	query.After = all[0].Cursor()

	rest, err := repo.ListLearnable(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []PositionEvaluation{all[1], all[2]}, rest)
}

// TestRepository_ListLearnable_ZeroCursorStartsAtTheBeginning pins the zero value's meaning, which
// the refill path relies on to start and to wrap a sweep.
func TestRepository_ListLearnable_ZeroCursorStartsAtTheBeginning(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:3]
	require.NoError(t, repo.AddPositions(ctx, positions))

	query := testLearnableQuery(12, 24, 10)
	query.After = LearnableCursor{}

	results, err := repo.ListLearnable(ctx, query)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

func TestRepository_EvaluatedPositions_OnlyReturnsLearnedBoards(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	positions := othello.PrecomputedPositions12()[:2]
	learned, unlearned := positions[0], positions[1]

	require.NoError(t, repo.AddPositions(ctx, positions))
	require.NoError(t, repo.SaveEvaluation(ctx, learned, Evaluation{Level: 20, Score: 5}))

	results, err := repo.EvaluatedPositions(ctx, 12)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, learned, results[0].Position)
	require.Equal(t, Evaluation{Level: 20, Score: 5}, results[0].Evaluation)

	require.NotContains(t, results, PositionEvaluation{Position: unlearned})
}

func TestRepository_EvaluatedPositions_FiltersByDiscCount(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position12 := testPosition(t, 12)
	position13 := testPosition(t, 13)
	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position12, position13}))
	require.NoError(t, repo.SaveEvaluation(ctx, position12, Evaluation{Level: 20, Score: 0}))
	require.NoError(t, repo.SaveEvaluation(ctx, position13, Evaluation{Level: 20, Score: 0}))

	results, err := repo.EvaluatedPositions(ctx, 12)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, position12, results[0].Position)
}

func TestRepository_EvaluatedPositions_Empty(t *testing.T) {
	repo := testRepository(t)

	results, err := repo.EvaluatedPositions(context.Background(), 12)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestRepository_Stats(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()

	position12s := testDistinctPositions(t, 12, 2)
	position12a, position12b := position12s[0], position12s[1]
	position13 := testPosition(t, 13)

	require.NoError(t, repo.AddPositions(ctx, []othello.NormalizedPosition{position12a, position12b, position13}))
	require.NoError(t, repo.SaveEvaluation(ctx, position12a, Evaluation{Level: 20, Score: 0}))

	stats, err := repo.Stats(ctx)
	require.NoError(t, err)

	require.Contains(t, stats, LevelStat{DiscCount: 12, Level: 0, Count: 1})
	require.Contains(t, stats, LevelStat{DiscCount: 12, Level: 20, Count: 1})
	require.Contains(t, stats, LevelStat{DiscCount: 13, Level: 0, Count: 1})
}
