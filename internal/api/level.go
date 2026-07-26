package api

// TargetLevel returns the edax search level a board with discCount discs
// should be learned to. It takes discCount so the mapping can vary by disc
// count in the future without changing callers; today every disc count
// maps to the same level.
func TargetLevel(discCount int) int {
	return 24
}
