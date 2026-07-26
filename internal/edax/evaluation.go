package edax

// Evaluation is a single edax search result for a position: the score and
// the depth/confidence the search reached it at, plus edax's principal
// variation of best moves.
//
// BestMoves is never persisted to the DB (see internal/db.Evaluation) — the
// book only stores the score; best moves are derived on demand by
// minimaxing over child scores instead. It's kept here for fidelity with
// what edax actually reported.
type Evaluation struct {
	Depth      int
	Confidence int
	Score      int
	BestMoves  []int
}
