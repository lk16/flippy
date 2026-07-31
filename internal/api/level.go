package api

// TargetLevel returns the edax search level to use for a board with discCount discs. The 12-disc
// boards are searched deepest, since every board below them is backfilled by minimaxing up from their
// evaluations; boards beyond 12 discs are only ever looked at directly, so they use a shallower level.
func TargetLevel(discCount int) int {
	if discCount > 12 {
		return 16
	}
	return 24
}
