package edax

// noSelectivity is the selectivity index meaning "full-width, no ProbCut" (edax's NO_SELECTIVITY).
const noSelectivity = 5

// selectivityPercent maps a selectivity index to the confidence percentage edax prints after "@"
// (selectivity_table, search.c:104-111).
var selectivityPercent = [6]int{73, 87, 95, 98, 99, 100}

// boardSquares is the number of squares on the board, so empties = boardSquares - discCount.
const boardSquares = 64

// MaxLevel is the highest edax search level search_global_init defines behavior for.
const MaxLevel = 60

// SearchParams returns the (depth, confidence) an edax search at level reports for a board with
// discCount discs. Direct port of search_global_init (search.c:161-346), the same mapping
// wasm/edax-eval/src/search.rs ports as depth_and_selectivity; confidence is the selectivity
// percentage edax prints, so 100 means full-width rather than "solved".
//
// The pair is what the boards table used to store per row: it is fully determined by
// (discCount, level), so the columns were dropped and this is computed on read instead.
func SearchParams(discCount, level int) (depth, confidence int) {
	empties := boardSquares - discCount

	depth, selectivity := depthAndSelectivity(level, empties)
	return depth, selectivityPercent[selectivity]
}

// IsFinal reports whether a search at level on a board with discCount discs reaches the end of the
// game full-width, making its score the game-theoretic result: no deeper search can change it.
func IsFinal(discCount, level int) bool {
	depth, confidence := SearchParams(discCount, level)
	return depth == boardSquares-discCount && confidence == 100
}

// depthAndSelectivity returns the search depth and selectivity index for a search at level with
// empties empty squares. Levels above MaxLevel behave like MaxLevel (exact solve, full width).
func depthAndSelectivity(level, empties int) (depth, selectivity int) {
	depth, selectivity = empties, noSelectivity

	switch {
	case level <= 0:
		depth = 0
	case level <= 10:
		if empties > 2*level {
			depth = level
		}
	case level <= 12:
		switch {
		case empties <= 21:
		case empties <= 24:
			selectivity = 3
		default:
			depth, selectivity = level, 0
		}
	case level <= 18:
		switch {
		case empties <= 21:
		case empties <= 24:
			selectivity = 3
		case empties <= 27:
			selectivity = 1
		default:
			depth, selectivity = level, 0
		}
	case level <= 21:
		switch {
		case empties <= 24:
		case empties <= 27:
			selectivity = 3
		case empties <= 30:
			selectivity = 1
		default:
			depth, selectivity = level, 0
		}
	case level <= 24:
		switch {
		case empties <= 24:
		case empties <= 27:
			selectivity = 4
		case empties <= 30:
			selectivity = 2
		case empties <= 33:
			selectivity = 0
		default:
			depth, selectivity = level, 0
		}
	case level <= 27:
		switch {
		case empties <= 27:
		case empties <= 30:
			selectivity = 3
		case empties <= 33:
			selectivity = 1
		default:
			depth, selectivity = level, 0
		}
	case level < 30:
		switch {
		case empties <= 27:
		case empties <= 30:
			selectivity = 4
		case empties <= 33:
			selectivity = 2
		case empties <= 36:
			selectivity = 0
		default:
			depth, selectivity = level, 0
		}
	case level <= 31:
		switch {
		case empties <= 30:
		case empties <= 33:
			selectivity = 3
		case empties <= 36:
			selectivity = 1
		default:
			depth, selectivity = level, 0
		}
	case level <= 33:
		switch {
		case empties <= 30:
		case empties <= 33:
			selectivity = 4
		case empties <= 36:
			selectivity = 2
		case empties <= 39:
			selectivity = 0
		default:
			depth, selectivity = level, 0
		}
	case level <= 35:
		switch {
		case empties <= 30:
		case empties <= 33:
			selectivity = 4
		case empties <= 36:
			selectivity = 3
		case empties <= 39:
			selectivity = 1
		default:
			depth, selectivity = level, 0
		}
	case level < MaxLevel:
		switch {
		case empties <= level-6:
		case empties <= level-3:
			selectivity = 4
		case empties <= level:
			selectivity = 3
		case empties <= level+3:
			selectivity = 2
		case empties <= level+6:
			selectivity = 1
		case empties <= level+9:
			selectivity = 0
		default:
			depth, selectivity = level, 0
		}
	}
	// level >= MaxLevel: exact solve, full width — the initial values, unchanged.

	return depth, selectivity
}
