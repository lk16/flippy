package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetLevel(t *testing.T) {
	require.Equal(t, 32, TargetLevel(12))
	require.Equal(t, 32, TargetLevel(16))
	require.Equal(t, 30, TargetLevel(17))
	require.Equal(t, 30, TargetLevel(20))
	require.Equal(t, 28, TargetLevel(21))
	require.Equal(t, 28, TargetLevel(30))
	require.Equal(t, 28, TargetLevel(64))
}

// TestTargetLevelTiers_MatchTargetLevel guards the contract handleLevelConfig relies on: the tiers
// served to the frontend must reproduce TargetLevel for every disc count, so the frontend's target
// for a board is exactly the one handleAnalyzeRequest clamps its requests to.
func TestTargetLevelTiers_MatchTargetLevel(t *testing.T) {
	tiers := TargetLevelTiers()
	require.NotEmpty(t, tiers)
	require.Equal(t, 64, tiers[len(tiers)-1].MaxDiscs, "last tier must cover a full board")

	for discCount := range 65 {
		var want int
		for _, tier := range tiers {
			if discCount <= tier.MaxDiscs {
				want = tier.Level
				break
			}
		}
		require.Equal(t, want, TargetLevel(discCount), "disc count %d", discCount)
	}
}

// TestTargetLevelTiers_ReturnsACopy makes sure a caller cannot rewrite the table TargetLevel reads.
func TestTargetLevelTiers_ReturnsACopy(t *testing.T) {
	TargetLevelTiers()[0].Level = 1
	require.Equal(t, 32, TargetLevel(4))
}
