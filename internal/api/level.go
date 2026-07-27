package api

// TargetLevel returns the edax search level a board with discCount discs
// should be learned to. The 12-disc leaves (see book.LeafDiscs) get the
// deepest search, since every position below them is backfilled from their
// evaluation; boards beyond that are only looked at directly, so a shallower
// level is enough.
func TargetLevel(discCount int) int {
	if discCount > 12 {
		return 16
	}
	return 24
}
