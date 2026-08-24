package edax

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSearchParams_MatchesSearchGlobalInit checks a cross-section of the C table
// (search_global_init, search.c:161-346), the same cases the Rust port is spot-checked against
// (wasm/edax-eval/src/search.rs), so the two ports cannot drift apart silently.
func TestSearchParams_MatchesSearchGlobalInit(t *testing.T) {
	tests := []struct {
		empties        int
		level          int
		wantDepth      int
		wantConfidence int
	}{
		// level 0: depth 0 always
		{0, 0, 0, 100},
		{30, 0, 0, 100},
		// level <= 10: exact solve when empties <= 2*level, else depth = level
		{8, 5, 8, 100},
		{11, 5, 5, 100},
		{20, 10, 20, 100},
		{21, 10, 10, 100},
		// level <= 12
		{21, 12, 21, 100},
		{22, 12, 22, 98},
		{25, 12, 12, 73},
		// level <= 18
		{21, 15, 21, 100},
		{22, 15, 22, 98},
		{25, 15, 25, 87},
		{28, 15, 15, 73},
		// level <= 21
		{24, 20, 24, 100},
		{27, 20, 27, 98},
		{30, 20, 30, 87},
		{31, 20, 20, 73},
		// level <= 24
		{24, 23, 24, 100},
		{27, 23, 27, 99},
		{30, 23, 30, 95},
		{33, 23, 33, 73},
		{34, 23, 23, 73},
		// level <= 27
		{27, 26, 27, 100},
		{30, 26, 30, 98},
		{33, 26, 33, 87},
		{34, 26, 26, 73},
		// level < 30
		{27, 28, 27, 100},
		{30, 28, 30, 99},
		{33, 28, 33, 95},
		{36, 28, 36, 73},
		{37, 28, 28, 73},
		// level <= 31
		{30, 31, 30, 100},
		{33, 31, 33, 98},
		{36, 31, 36, 87},
		{37, 31, 31, 73},
		// level <= 33
		{30, 32, 30, 100},
		{33, 32, 33, 99},
		{36, 32, 36, 95},
		{39, 32, 39, 73},
		{40, 32, 32, 73},
		// level <= 35
		{30, 34, 30, 100},
		{33, 34, 33, 99},
		{36, 34, 36, 98},
		{39, 34, 39, 87},
		{40, 34, 34, 73},
		// level < 60: thresholds are level±{6,3,0,3,6,9}
		{34, 40, 34, 100},
		{37, 40, 37, 99},
		{40, 40, 40, 98},
		{43, 40, 43, 95},
		{46, 40, 46, 87},
		{49, 40, 49, 73},
		{50, 40, 40, 73},
		// level 60 and above: exact solve, full width
		{0, 60, 0, 100},
		{30, 60, 30, 100},
		{60, 60, 60, 100},
		{60, 61, 60, 100},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("empties=%d/level=%d", tt.empties, tt.level), func(t *testing.T) {
			depth, confidence := SearchParams(boardSquares-tt.empties, tt.level)
			require.Equal(t, tt.wantDepth, depth)
			require.Equal(t, tt.wantConfidence, confidence)
		})
	}
}

// TestSearchParams_BookTargets covers the (depth, confidence) pairs the book actually stores: every
// disc-count tier at its target level (see api.TargetLevel).
func TestSearchParams_BookTargets(t *testing.T) {
	tests := []struct {
		discCount      int
		level          int
		wantDepth      int
		wantConfidence int
	}{
		{12, 40, 40, 73},
		{13, 40, 40, 73},
		{14, 36, 36, 73},
		{16, 36, 36, 73},
		{17, 34, 34, 73},
		{20, 34, 34, 73},
		{21, 32, 32, 73},
		{24, 32, 32, 73},
		{25, 32, 39, 73},
		{27, 32, 37, 73},
		{28, 32, 36, 95},
		{30, 32, 34, 95},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("discs=%d/level=%d", tt.discCount, tt.level), func(t *testing.T) {
			depth, confidence := SearchParams(tt.discCount, tt.level)
			require.Equal(t, tt.wantDepth, depth)
			require.Equal(t, tt.wantConfidence, confidence)
		})
	}
}

// TestSearchParams_ConfidenceIsAnEdaxPercentage checks that every level/disc-count pair reports one
// of the six percentages edax's selectivity table defines.
func TestSearchParams_ConfidenceIsAnEdaxPercentage(t *testing.T) {
	allowed := map[int]bool{73: true, 87: true, 95: true, 98: true, 99: true, 100: true}

	for discCount := 4; discCount <= boardSquares; discCount++ {
		for level := range MaxLevel + 1 {
			depth, confidence := SearchParams(discCount, level)
			require.True(t, allowed[confidence], "discs=%d level=%d confidence=%d", discCount, level, confidence)
			require.LessOrEqual(t, depth, boardSquares-discCount, "discs=%d level=%d", discCount, level)
			require.GreaterOrEqual(t, depth, 0, "discs=%d level=%d", discCount, level)
		}
	}
}

func TestIsFinal(t *testing.T) {
	tests := []struct {
		discCount int
		level     int
		want      bool
	}{
		{44, 10, true},  // 20 empties, solved outright from level 10 up
		{44, 28, true},  // ... and every deeper level too
		{44, 8, false},  // depth 8 < 20 empties
		{12, 40, false}, // 52 empties: a 40-ply midgame search
		{28, 32, false}, // exact depth but selective (95%), so not final
		{34, 32, true},  // 30 empties, full-width exact solve
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("discs=%d/level=%d", tt.discCount, tt.level), func(t *testing.T) {
			require.Equal(t, tt.want, IsFinal(tt.discCount, tt.level))
		})
	}
}

func TestAlignLevel(t *testing.T) {
	tests := []struct {
		name      string
		discCount int
		level     int
		want      int
	}{
		{"depth-limited, parity already matches", 12, 40, 40},
		{"depth-limited, parity mismatch", 13, 40, 41},
		{"depth-limited, parity mismatch the other way", 12, 41, 42},
		{"interactive level at an odd disc count", 13, 16, 17},
		{"interactive level at an even disc count", 14, 16, 16},
		// 39 empties searched to the end: the depth is the empty count, so the parity already
		// alternates per ply and raising the level would only buy a slower identical search.
		{"search runs the game out", 25, 32, 32},
		{"exact solve", 44, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, AlignLevel(tt.discCount, tt.level))
		})
	}
}

// TestAlignLevel_MatchesDiscParity is the property the rule exists for: after alignment a
// depth-limited search always searches as many ply as the disc count's parity, so adjacent plies
// never share a depth parity.
func TestAlignLevel_MatchesDiscParity(t *testing.T) {
	for discCount := 4; discCount < boardSquares; discCount++ {
		for level := 1; level <= MaxLevel; level++ {
			aligned := AlignLevel(discCount, level)
			require.Contains(t, []int{level, level + 1}, aligned, "discs=%d level=%d", discCount, level)

			depth, _ := SearchParams(discCount, aligned)
			if depth == boardSquares-discCount {
				continue
			}
			require.Equal(t, discCount%2, depth%2, "discs=%d level=%d", discCount, level)
		}
	}
}
