package edax

// Evaluation is a single edax search result: score, depth/confidence, and best moves. BestMoves is
// never persisted to the DB; the book only stores the score and derives best moves via minimax instead.
type Evaluation struct {
	Depth      int
	Confidence int
	Score      int
	BestMoves  []int
}
