package gui

import (
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/models"
)

const (
	BoardWidthPx  = 600
	BoardHeightPx = 600

	SquareSize          = BoardWidthPx / 8
	DiscRadius          = SquareSize/2 - 5
	MoveIndicatorRadius = SquareSize / 8
)

type windowDrawer struct {
	controller *Controller
}

func newWindowDrawer(controller *Controller) *windowDrawer {
	return &windowDrawer{controller: controller}
}

func (w *windowDrawer) turnToColor(turn int) rl.Color {
	switch turn {
	case models.WHITE:
		return rl.White
	case models.BLACK:
		return rl.Black
	default:
		panic("invalid turn")
	}
}

func (w *windowDrawer) draw() {
	args := w.controller.GetDrawArgs()
	board := args.Board

	rl.BeginDrawing()

	backgroundColor := rl.NewColor(0, 128, 0, 255)
	rl.ClearBackground(backgroundColor)

	for index := range 64 {
		switch board.GetSquare(index) {
		case models.WHITE:
			w.drawDisc(index, w.turnToColor(models.WHITE))
		case models.BLACK:
			w.drawDisc(index, w.turnToColor(models.BLACK))
		case models.EMPTY:
			w.drawEmptySquare(index, args)
		}
	}

	rl.EndDrawing()
}

func (w *windowDrawer) drawEmptySquare(index int, args *DrawArgs) {
	board := args.Board

	if !board.IsValidMove(index) {
		return
	}

	color := w.turnToColor(board.Turn())

	eval, evalFound := args.SquareEvaluations[index]
	if !evalFound || !args.ShowEvaluations {
		w.drawMoveIndicator(index, color)
		return
	}

	w.drawEvaluationScore(index, eval.Score, color)

	if args.ShowSearchDepth {
		w.drawEvaluationDepth(index, eval.Depth, color)
	}

	if eval.IsBest {
		w.drawBestEvaluationIndicator(index, color)
	}
}

func (w *windowDrawer) getSquareCenter(index int) (int32, int32) {
	col := index % 8
	row := index / 8

	x := col*SquareSize + SquareSize/2
	y := row*SquareSize + SquareSize/2

	return int32(x), int32(y) //nolint:gosec
}

func (w *windowDrawer) drawDisc(index int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)
	rl.DrawCircle(centerX, centerY, DiscRadius, color)
}

func (w *windowDrawer) drawMoveIndicator(index int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)
	rl.DrawCircle(centerX, centerY, MoveIndicatorRadius, color)
}

func (w *windowDrawer) drawEvaluationScore(index int, score int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)

	fontSize := int32(30)

	text := strconv.Itoa(score)
	textWidth := rl.MeasureText(text, fontSize)
	textX := centerX - textWidth/2
	textY := centerY - fontSize/2

	rl.DrawText(text, textX, textY, fontSize, color)
}

func (w *windowDrawer) drawEvaluationDepth(index int, depth int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)

	fontSize := int32(12)

	text := strconv.Itoa(depth)
	textWidth := rl.MeasureText(text, fontSize)
	textX := centerX - textWidth/2 + SquareSize/8
	textY := centerY - fontSize/2 + SquareSize/4

	rl.DrawText(text, textX, textY, fontSize, color)
}

func (w *windowDrawer) drawBestEvaluationIndicator(index int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)
	center := rl.Vector2{X: float32(centerX), Y: float32(centerY)}
	rl.DrawRing(center, DiscRadius-1, DiscRadius, 0, 360, 40, color)
}
