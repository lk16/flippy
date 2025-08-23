package gui

import (
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

type Window struct {
}

func NewWindow() *Window {
	return &Window{}
}

func (w *Window) Run() {
	rl.SetTraceLogLevel(rl.LogError)

	rl.InitWindow(BoardWidthPx, BoardHeightPx, "Flippy")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	for !rl.WindowShouldClose() {
		w.draw()
	}
}

func (w *Window) draw() {
	rl.BeginDrawing()

	backgroundColor := rl.NewColor(0, 128, 0, 255)
	rl.ClearBackground(backgroundColor)

	board := models.NewBoardStart()

	for index := range 64 {
		switch board.GetSquare(index) {
		case models.WHITE:
			w.drawDisc(index, rl.White)
		case models.BLACK:
			w.drawDisc(index, rl.Black)
		case models.EMPTY:
			if board.IsValidMove(index) {
				if board.Turn() == models.WHITE {
					w.drawMoveIndicator(index, rl.White)
				} else {
					w.drawMoveIndicator(index, rl.Black)
				}
			}
		}
	}

	rl.EndDrawing()
}

func (w *Window) getSquareCenter(index int) (int32, int32) {
	col := index % 8
	row := index / 8

	x := col*SquareSize + SquareSize/2
	y := row*SquareSize + SquareSize/2

	return int32(x), int32(y) //nolint:gosec
}

func (w *Window) drawDisc(index int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)
	rl.DrawCircle(centerX, centerY, DiscRadius, color)
}

func (w *Window) drawMoveIndicator(index int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)
	rl.DrawCircle(centerX, centerY, MoveIndicatorRadius, color)
}
