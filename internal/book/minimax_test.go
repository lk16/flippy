package book

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/othello"
)

// allNormalizedPositionsAt returns every distinct normalized position with exactly discs discs
// reachable from the starting position.
func allNormalizedPositionsAt(t *testing.T, discs int) []othello.Position {
	t.Helper()

	seen := make(map[othello.Position]bool)
	var result []othello.Position

	visited := make(map[othello.Position]bool)
	var visit func(b othello.Position)
	visit = func(b othello.Position) {
		if visited[b] {
			return
		}
		visited[b] = true

		if b.CountDiscs() == discs {
			norm := b.Normalize().Position()
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

	visit(othello.NewStartPosition())
	return result
}

func TestBuildCache_SingleChild(t *testing.T) {
	root := othello.NewStartPosition()
	// The 4 opening moves are rotations of each other, so one leaf covers all of them.
	child := root.Children()[0].Normalize().Position()

	cache := buildCache(root, 5, map[othello.Position]int{child: 7})

	require.Equal(t, map[othello.Position]int{root.Normalize().Position(): -7}, cache)
}

func TestBuildCache_NoLeaves(t *testing.T) {
	root := othello.NewStartPosition()
	cache := buildCache(root, 5, nil)
	require.Empty(t, cache)
}

func TestBuildCache_PartialLeavesGiveNoResult(t *testing.T) {
	root := othello.NewStartPosition()
	leaves6 := allNormalizedPositionsAt(t, 6)
	require.Greater(t, len(leaves6), 1)

	partial := map[othello.Position]int{leaves6[0]: 3}
	cache := buildCache(root, 6, partial)
	require.Empty(t, cache)
}

func TestBuildCache_FullLeavesCoverAllAncestors(t *testing.T) {
	root := othello.NewStartPosition()
	leaves6 := allNormalizedPositionsAt(t, 6)

	full := make(map[othello.Position]int, len(leaves6))
	for i, b := range leaves6 {
		full[b] = i
	}

	cache := buildCache(root, 6, full)

	for position := range cache {
		require.Less(t, position.CountDiscs(), 6)
	}
	require.Contains(t, cache, root.Normalize().Position())

	for _, b := range allNormalizedPositionsAt(t, 5) {
		require.Contains(t, cache, b)
	}
}

func TestBuildCache_ForcedPass(t *testing.T) {
	// Reachable position where the player to move has no legal move, but the opponent does after the pass.
	position, err := othello.ParsePosition("00000038180c00010000000000030204")
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	passed, err := position.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.True(t, passed.HasMoves())

	children := passed.Children()
	require.NotEmpty(t, children)

	leaves := make(map[othello.Position]int)
	for i, child := range children {
		leaves[child.Normalize().Position()] = 10 + i
	}

	cache := buildCache(position, position.CountDiscs()+1, leaves)

	passedBest := noValue
	for _, child := range children {
		if score := -leaves[child.Normalize().Position()]; score > passedBest {
			passedBest = score
		}
	}
	want := -passedBest

	got, ok := cache[position.Normalize().Position()]
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestBuildCache_ForcedPassUndeterminedSuccessorExcludesBoard(t *testing.T) {
	position, err := othello.ParsePosition("00000038180c00010000000000030204")
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	// No leaves at all: the passed-to position's children can't be resolved, so the forced-pass
	// position itself must be excluded too.
	cache := buildCache(position, position.CountDiscs()+1, nil)
	require.NotContains(t, cache, position.Normalize().Position())
}

func TestBuildCache_GameOver(t *testing.T) {
	// Reachable position where the game is over: both players are out of moves.
	position, err := othello.ParsePosition("3fb0888090a0c080c04f777f6f5f3f7f")
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	passed, err := position.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.False(t, passed.HasMoves())

	cache := buildCache(position, position.CountDiscs()+1, nil)

	got, ok := cache[position.Normalize().Position()]
	require.True(t, ok)
	require.Equal(t, position.FinalScore(), got)
}

func TestMinimaxValue_ForcedPassAtLeafBoundary(t *testing.T) {
	// The forced-pass position is itself at the leaf boundary; leaves can only hold the passed-to
	// position (edax never evaluates a no-legal-move position), so the value must be its negation.
	position, err := othello.ParsePosition("00000038180c00010000000000030204")
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	passed, err := position.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.True(t, passed.HasMoves())

	leaves := map[othello.Position]int{passed.Normalize().Position(): 9}

	entry := minimaxValue(position, position.CountDiscs(), leaves, make(map[othello.Position]cacheEntry), make(map[othello.Position]bool))

	require.True(t, entry.ok)
	require.Equal(t, -9, entry.score)
}

func TestMinimaxValue_ForcedPassAtLeafBoundaryUnlearnedSuccessorIsUndetermined(t *testing.T) {
	position, err := othello.ParsePosition("00000038180c00010000000000030204")
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	// The passed-to position isn't in leaves, so the forced-pass position can't be determined either.
	entry := minimaxValue(position, position.CountDiscs(), nil, make(map[othello.Position]cacheEntry), make(map[othello.Position]bool))

	require.False(t, entry.ok)
}

func TestMinimaxValue_GameOverAtLeafBoundary(t *testing.T) {
	// Game-over position at the leaf boundary: minimaxPass's game-over short-circuit must fire
	// before the leafDiscs lookup misses on a position that was never in leaves.
	position, err := othello.ParsePosition("3fb0888090a0c080c04f777f6f5f3f7f")
	require.NoError(t, err)
	require.False(t, position.HasMoves())

	passed, err := position.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.False(t, passed.HasMoves())

	entry := minimaxValue(position, position.CountDiscs(), nil, make(map[othello.Position]cacheEntry), make(map[othello.Position]bool))

	require.True(t, entry.ok)
	require.Equal(t, position.FinalScore(), entry.score)
}

func TestBuildCache_CycleDetectionPanics(t *testing.T) {
	// Simulate re-entering minimaxValue on a position still being resolved (the real game graph
	// has no cycles).
	position := othello.NewStartPosition()
	visiting := map[othello.Position]bool{position.Normalize().Position(): true}

	require.Panics(t, func() {
		minimaxValue(position, 12, nil, make(map[othello.Position]cacheEntry), visiting)
	})
}
