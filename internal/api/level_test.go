package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetLevel(t *testing.T) {
	require.Equal(t, 40, TargetLevel(12))
	require.Equal(t, 40, TargetLevel(13))
	require.Equal(t, 36, TargetLevel(14))
	require.Equal(t, 36, TargetLevel(16))
	require.Equal(t, 34, TargetLevel(17))
	require.Equal(t, 34, TargetLevel(20))
	require.Equal(t, 32, TargetLevel(21))
	require.Equal(t, 32, TargetLevel(30))
	require.Equal(t, 32, TargetLevel(64))
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

// TestIsBookQuality covers the level floor handleSubmitJobResult applies to every submission:
// only a search at least as deep as the board's target level -- or one that already ran the game
// out -- may reach the DB.
func TestIsBookQuality(t *testing.T) {
	tests := []struct {
		name      string
		discCount int
		level     int
		want      bool
	}{
		{"below target", 14, PriorityLevel, false},
		{"one rung below target", 14, TargetLevel(14) - 2, false},
		{"at target", 14, TargetLevel(14), true},
		{"above target", 14, TargetLevel(14) + 2, true},
		{"at target, deepest tier", 30, TargetLevel(30), true},
		// 52 discs is past MaxSavableDiscs, so the disc-count check keeps it out of the DB anyway,
		// but it is the shape the IsFinal clause exists for: a shallow search that is still the
		// game-theoretic result, which no deeper level could improve on.
		{"below target but final", 52, 12, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isBookQuality(tt.discCount, tt.level))
		})
	}
}

// TestTargetLevelTiers_ReturnsACopy makes sure a caller cannot rewrite the table TargetLevel reads.
func TestTargetLevelTiers_ReturnsACopy(t *testing.T) {
	TargetLevelTiers()[0].Level = 1
	require.Equal(t, 40, TargetLevel(4))
}
