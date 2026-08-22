package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

func TestRebuildIfVersionChanged(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	// Initial build happens even with no version key: -1 never equals the read version (0).
	require.False(t, s.cache.Built())
	require.EqualValues(t, 0, s.rebuildIfVersionChanged(ctx, -1))
	require.True(t, s.cache.Built())

	// Learn every leaf child of an 11-disc position, which makes it minimax-resolvable...
	position11 := testPosition(t, book.LeafDiscs-1)
	for _, child := range position11.Position().Children() {
		normalized := child.Normalize()
		require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{normalized}))
		require.NoError(t, s.repo.SaveEvaluation(ctx, normalized,
			db.Evaluation{Level: TargetLevel(book.LeafDiscs), Score: 3}))
	}

	// ...but with an unchanged version nothing rebuilds.
	require.EqualValues(t, 0, s.rebuildIfVersionChanged(ctx, 0))
	_, ok := s.cache.Get(position11.Position())
	require.False(t, ok)

	// A bumped version triggers the rebuild that picks it up.
	require.NoError(t, s.bumpBookVersion(ctx))
	require.EqualValues(t, 1, s.rebuildIfVersionChanged(ctx, 0))
	_, ok = s.cache.Get(position11.Position())
	require.True(t, ok)
}
