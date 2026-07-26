package book

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// allNormalizedBoardsAt returns every distinct normalized board with
// exactly discs discs reachable from the starting position.
func allNormalizedBoardsAt(t *testing.T, discs int) []othello.Board {
	t.Helper()

	seen := make(map[othello.Board]bool)
	var result []othello.Board

	visited := make(map[othello.Board]bool)
	var visit func(b othello.Board)
	visit = func(b othello.Board) {
		if visited[b] {
			return
		}
		visited[b] = true

		if b.CountDiscs() == discs {
			norm := b.Normalize().Board()
			if !seen[norm] {
				seen[norm] = true
				result = append(result, norm)
			}
			return
		}

		if !b.HasMoves() {
			next, err := b.DoMove(othello.PassMove)
			require.NoError(t, err)
			visit(next)
			return
		}

		for _, child := range b.Children() {
			visit(child)
		}
	}

	visit(othello.NewBoardStart())
	return result
}

func TestBuildCache_SingleChild(t *testing.T) {
	root := othello.NewBoardStart()
	// The 4 opening moves are rotations of each other, so they all
	// normalize to the same board: exactly one leaf is needed.
	child := root.Children()[0].Normalize().Board()

	cache := buildCache(root, 5, map[othello.Board]int{child: 7})

	require.Equal(t, map[othello.Board]int{root.Normalize().Board(): -7}, cache)
}

func TestBuildCache_NoLeaves(t *testing.T) {
	root := othello.NewBoardStart()
	cache := buildCache(root, 5, nil)
	require.Empty(t, cache)
}

func TestBuildCache_PartialLeavesGiveNoResult(t *testing.T) {
	root := othello.NewBoardStart()
	leaves6 := allNormalizedBoardsAt(t, 6)
	require.Greater(t, len(leaves6), 1)

	partial := map[othello.Board]int{leaves6[0]: 3}
	cache := buildCache(root, 6, partial)
	require.Empty(t, cache)
}

func TestBuildCache_FullLeavesCoverAllAncestors(t *testing.T) {
	root := othello.NewBoardStart()
	leaves6 := allNormalizedBoardsAt(t, 6)

	full := make(map[othello.Board]int, len(leaves6))
	for i, b := range leaves6 {
		full[b] = i
	}

	cache := buildCache(root, 6, full)

	for board := range cache {
		require.Less(t, board.CountDiscs(), 6)
	}
	require.Contains(t, cache, root.Normalize().Board())

	for _, b := range allNormalizedBoardsAt(t, 5) {
		require.Contains(t, cache, b)
	}
}

func TestBuildCache_ForcedPass(t *testing.T) {
	// Found via BFS from the starting position: black has no legal move
	// here, but white does after the pass.
	board, err := othello.ParseBoard("00000038180c00010000000000030204-b")
	require.NoError(t, err)
	require.False(t, board.HasMoves())

	passed, err := board.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.True(t, passed.HasMoves())

	children := passed.Children()
	require.NotEmpty(t, children)

	leaves := make(map[othello.Board]int)
	for i, child := range children {
		leaves[child.Normalize().Board()] = 10 + i
	}

	cache := buildCache(board, board.CountDiscs()+1, leaves)

	passedBest := noValue
	for _, child := range children {
		if score := -leaves[child.Normalize().Board()]; score > passedBest {
			passedBest = score
		}
	}
	want := -passedBest

	got, ok := cache[board.Normalize().Board()]
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestBuildCache_ForcedPassUndeterminedSuccessorExcludesBoard(t *testing.T) {
	board, err := othello.ParseBoard("00000038180c00010000000000030204-b")
	require.NoError(t, err)
	require.False(t, board.HasMoves())

	// No leaves at all: the passed-to position's children can't be
	// resolved, so the forced-pass board itself must be excluded too.
	cache := buildCache(board, board.CountDiscs()+1, nil)
	require.NotContains(t, cache, board.Normalize().Board())
}

func TestBuildCache_GameOver(t *testing.T) {
	// Found by always playing the first available move from the start:
	// the game ends (both players out of moves) with a full board.
	board, err := othello.ParseBoard("3fb0888090a0c080c04f777f6f5f3f7f-b")
	require.NoError(t, err)
	require.False(t, board.HasMoves())

	passed, err := board.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.False(t, passed.HasMoves())

	cache := buildCache(board, board.CountDiscs()+1, nil)

	got, ok := cache[board.Normalize().Board()]
	require.True(t, ok)
	require.Equal(t, board.FinalScore(), got)
}

func TestBuildCache_CycleDetectionPanics(t *testing.T) {
	// minimaxValue must never be re-entered for a board it's still
	// resolving; simulate that directly rather than trying to construct a
	// real cyclic position (the game graph has none).
	board := othello.NewBoardStart()
	visiting := map[othello.Board]bool{board.Normalize().Board(): true}

	require.Panics(t, func() {
		minimaxValue(board, 12, nil, make(map[othello.Board]cacheEntry), visiting)
	})
}
