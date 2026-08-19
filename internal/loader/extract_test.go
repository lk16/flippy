package loader

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

var testPosition = othellotest.Position

func TestExtractPositions_PlayedLineWithinRange(t *testing.T) {
	// A known-legal move sequence that reaches 14 discs with no passes.
	game, err := othello.NewGameFromMoves([]int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2})
	require.NoError(t, err)
	require.Equal(t, 14, game.Position().CountDiscs())

	positions := ExtractPositions([]*othello.Game{game})
	require.NotEmpty(t, positions)

	for _, b := range positions {
		require.True(t, b.HasMoves())
		require.GreaterOrEqual(t, b.CountDiscs(), book.LeafDiscs)
		require.LessOrEqual(t, b.CountDiscs(), book.MaxSavableDiscs)
	}
}

func TestExtractPositions_IncludesChildrenOfPlayedLine(t *testing.T) {
	start := testPosition(t, book.LeafDiscs).Position()
	game := othello.NewGameWithStart(start)

	positions := ExtractPositions([]*othello.Game{game})

	// start itself (LeafDiscs discs) plus its children (LeafDiscs+1 discs).
	require.Contains(t, positions, start.Normalize())

	childFound := false
	for _, child := range start.Children() {
		if containsBoard(positions, child.Normalize()) {
			childFound = true
		}
	}
	require.True(t, childFound, "expected at least one child of the start position to be extracted")
}

func TestExtractPositions_ExcludesBelowLeafDiscs(t *testing.T) {
	start := testPosition(t, book.LeafDiscs-1).Position()
	game := othello.NewGameWithStart(start)

	positions := ExtractPositions([]*othello.Game{game})

	require.NotContains(t, positions, start.Normalize())
}

func TestExtractPositions_ExcludesAboveMaxSavableDiscs(t *testing.T) {
	start := testPosition(t, book.MaxSavableDiscs+1).Position()
	game := othello.NewGameWithStart(start)

	positions := ExtractPositions([]*othello.Game{game})

	require.NotContains(t, positions, start.Normalize())
}

func TestExtractPositions_IncludesMaxSavableDiscsBoundary(t *testing.T) {
	start := testPosition(t, book.MaxSavableDiscs).Position()
	game := othello.NewGameWithStart(start)

	positions := ExtractPositions([]*othello.Game{game})

	require.Contains(t, positions, start.Normalize())
}

func TestExtractPositions_ExcludesBoardWithNoLegalMove(t *testing.T) {
	// Black holds row 0, white row 7, black to move: black has no legal move even though the
	// disc count (16) is within the savable range.
	position, err := othello.NewPosition(0xFF, 0xFF00000000000000)
	require.NoError(t, err)
	require.False(t, position.HasMoves(), "test position must have no legal move for this test to be meaningful")
	require.Equal(t, 16, position.CountDiscs())

	game := othello.NewGameWithStart(position)

	positions := ExtractPositions([]*othello.Game{game})

	require.NotContains(t, positions, position.Normalize())
}

func TestExtractPositions_DedupesAcrossGames(t *testing.T) {
	start := testPosition(t, book.LeafDiscs).Position()
	game1 := othello.NewGameWithStart(start)
	game2 := othello.NewGameWithStart(start)

	positions := ExtractPositions([]*othello.Game{game1, game2})

	count := 0
	for _, b := range positions {
		if b == start.Normalize() {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestExtractPositions_NoGames(t *testing.T) {
	require.Empty(t, ExtractPositions(nil))
}

func containsBoard(positions []othello.NormalizedPosition, target othello.NormalizedPosition) bool {
	for _, b := range positions {
		if b == target {
			return true
		}
	}
	return false
}
