package main

import (
	"testing"

	"github.com/lk16/flippy/internal/othello"
	"github.com/stretchr/testify/require"
)

func TestExplore_RecordsTargetDiscBoard(t *testing.T) {
	position := positionWithDiscs(t, targetDiscs)

	found := make(map[string]struct{})
	explore(position, make(map[othello.Position]bool), found)

	require.Contains(t, found, position.Normalize().String())
	require.Len(t, found, 1)
}

func TestExplore_SkipsAlreadyVisited(t *testing.T) {
	position := positionWithDiscs(t, targetDiscs)

	visited := map[othello.Position]bool{position: true}
	found := make(map[string]struct{})
	explore(position, visited, found)

	require.Empty(t, found)
}

func TestExplore_StopsWhenGameEndsEarly(t *testing.T) {
	// No legal moves for either player below targetDiscs discs: the game is over, so no
	// targetDiscs position can be reached.
	full := ^uint64(0)
	position, err := othello.NewPosition(full, 0)
	require.NoError(t, err)

	found := make(map[string]struct{})
	explore(position, make(map[othello.Position]bool), found)

	require.Empty(t, found)
}

func TestExplore_FromStart_MatchesPrecomputedCount(t *testing.T) {
	found := make(map[string]struct{})
	explore(othello.NewStartPosition(), make(map[othello.Position]bool), found)

	require.Len(t, found, 67245)
}

func TestExplore_FollowsForcedPassAtTargetDiscBoard(t *testing.T) {
	// A real 12-disc position where black (to move) must pass: the position itself is skipped (edax
	// can't evaluate it), but the passed-into position is a genuine targetDiscs position to keep.
	position, err := othello.ParsePosition("0000001c183000800000000000c04020")
	require.NoError(t, err)
	require.Equal(t, targetDiscs, position.CountDiscs())
	require.False(t, position.HasMoves(), "test position must have no legal move for this test to be meaningful")

	passed, err := position.DoMove(othello.PassMove)
	require.NoError(t, err)
	require.True(t, passed.HasMoves(), "passed-into position must have a legal move for this test to be meaningful")

	found := make(map[string]struct{})
	explore(position, make(map[othello.Position]bool), found)

	require.Equal(t, map[string]struct{}{passed.Normalize().String(): {}}, found)
}

// positionWithDiscs returns any legal position with exactly n discs, reached by
// repeatedly playing the first available legal move (or pass) from start.
func positionWithDiscs(t *testing.T, n int) othello.Position {
	t.Helper()

	position := othello.NewStartPosition()
	for position.CountDiscs() < n {
		if !position.HasMoves() {
			next, err := position.DoMove(othello.PassMove)
			require.NoError(t, err)
			position = next
			continue
		}

		children := position.Children()
		require.NotEmpty(t, children)
		position = children[0]
	}

	return position
}
