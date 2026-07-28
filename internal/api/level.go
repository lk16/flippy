package api

// TargetLevel returns the edax search level for discCount: deepest for the 12-disc leaves everything
// below is backfilled from, shallower beyond since those boards are only ever looked at directly.
func TargetLevel(discCount int) int {
	if discCount > 12 {
		return 16
	}
	return 24
}
