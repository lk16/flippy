package gui

import "github.com/lk16/flippy/api/internal/othello"

// DrawArgs contains arguments for drawing the window content.
type DrawArgs struct {
	// Board is the current board state
	Board othello.Board

	// Evaluations is a map of index to evaluation.
	SquareEvaluations map[int]*MoveEvaluation

	// ShowEvaluations decides if we show move evaluations
	ShowEvaluations bool

	// ShowSearchDepth decides if we show the depth of the search for evaluations
	ShowSearchDepth bool
}

// MoveEvaluation contains details about the evaluation of a single move.
type MoveEvaluation struct {
	// Score is the score for this square
	Score int

	// Depth is the computation depth for score
	Depth int

	// IsBest indicates if this square has the best score
	IsBest bool
}
