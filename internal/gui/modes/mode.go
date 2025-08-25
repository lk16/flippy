package modes

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/models"
)

type UIOPtions struct {
	// Evaluations is a map of index to evaluation.
	Evaluations map[int]int

	// BestEvaluation is the score of the best evaluation.
	BestEvaluation int
}

type Mode interface {
	// GetBoard returns the board.
	GetBoard() models.Board

	// OnMove is called when a move is made.
	OnMove(index int)

	// OnClick is called when a click is made.
	OnClick(button rl.MouseButton, x, y int)

	// OnKeyPress is called when a key is pressed.
	OnKeyPress(key int)

	// GetUIOptions returns the UI options.
	GetUIOptions() *UIOPtions

	// OnFrame is called when a frame is drawn.
	OnFrame()
}
