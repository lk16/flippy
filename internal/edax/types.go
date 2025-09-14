package edax

import "github.com/lk16/flippy/api/internal/models"

// Result contains an edax evaluation or an error.
type Result struct {
	Result *models.JobResult
	Err    error
}

// parsedLine is used by edax.parser.
type parsedLine struct {
	depth      int
	confidence int
	score      int
	bestMoves  []int
}
