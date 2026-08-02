package loader

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/othello"
	"github.com/lk16/flippy/internal/othello/othellotest"
)

// testBoard returns a NormalizedBoard reached by playing the first available
// legal move (or pass) from start until it has exactly discs discs.
var testBoard = othellotest.Board

func TestExtractBoards_PlayedLineWithinRange(t *testing.T) {
	// A real move sequence, known-legal (also used in wtb_test.go), that
	// reaches 14 discs with no passes.
	game, err := othello.NewGameFromMoves([]int{19, 18, 17, 9, 1, 0, 37, 43, 51, 2})
	require.NoError(t, err)
	require.Equal(t, 14, game.Board().CountDiscs())

	boards := ExtractBoards([]*othello.Game{game})
	require.NotEmpty(t, boards)

	for _, b := range boards {
		require.True(t, b.HasMoves())
		require.GreaterOrEqual(t, b.CountDiscs(), book.LeafDiscs)
		require.LessOrEqual(t, b.CountDiscs(), book.MaxSavableDiscs)
	}
}

func TestExtractBoards_IncludesChildrenOfPlayedLine(t *testing.T) {
	start := testBoard(t, book.LeafDiscs).Board()
	game := othello.NewGameWithStart(start)

	boards := ExtractBoards([]*othello.Game{game})

	// start itself (LeafDiscs discs) plus its children (LeafDiscs+1 discs).
	require.Contains(t, boards, start.Normalize())

	childFound := false
	for _, child := range start.Children() {
		if containsBoard(boards, child.Normalize()) {
			childFound = true
		}
	}
	require.True(t, childFound, "expected at least one child of the start board to be extracted")
}

func TestExtractBoards_ExcludesBelowLeafDiscs(t *testing.T) {
	start := testBoard(t, book.LeafDiscs-1).Board()
	game := othello.NewGameWithStart(start)

	boards := ExtractBoards([]*othello.Game{game})

	require.NotContains(t, boards, start.Normalize())
}

func TestExtractBoards_ExcludesAboveMaxSavableDiscs(t *testing.T) {
	start := testBoard(t, book.MaxSavableDiscs+1).Board()
	game := othello.NewGameWithStart(start)

	boards := ExtractBoards([]*othello.Game{game})

	require.NotContains(t, boards, start.Normalize())
}

func TestExtractBoards_IncludesMaxSavableDiscsBoundary(t *testing.T) {
	start := testBoard(t, book.MaxSavableDiscs).Board()
	game := othello.NewGameWithStart(start)

	boards := ExtractBoards([]*othello.Game{game})

	require.Contains(t, boards, start.Normalize())
}

func TestExtractBoards_ExcludesBoardWithNoLegalMove(t *testing.T) {
	// Black occupies all of row 0, white all of row 7, black to move: no
	// empty square is adjacent to a flippable run for black, so black has
	// no legal move even though disc count (16) is within the savable
	// range.
	board, err := othello.NewBoard(0xFF, 0xFF00000000000000, othello.Black)
	require.NoError(t, err)
	require.False(t, board.HasMoves(), "test board must have no legal move for this test to be meaningful")
	require.Equal(t, 16, board.CountDiscs())

	game := othello.NewGameWithStart(board)

	boards := ExtractBoards([]*othello.Game{game})

	require.NotContains(t, boards, board.Normalize())
}

func TestExtractBoards_DedupesAcrossGames(t *testing.T) {
	start := testBoard(t, book.LeafDiscs).Board()
	game1 := othello.NewGameWithStart(start)
	game2 := othello.NewGameWithStart(start)

	boards := ExtractBoards([]*othello.Game{game1, game2})

	count := 0
	for _, b := range boards {
		if b == start.Normalize() {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestExtractBoards_NoGames(t *testing.T) {
	require.Empty(t, ExtractBoards(nil))
}

func containsBoard(boards []othello.NormalizedBoard, target othello.NormalizedBoard) bool {
	for _, b := range boards {
		if b == target {
			return true
		}
	}
	return false
}
