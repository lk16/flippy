package gui

import (
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/gui/modes"
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
	mode modes.Mode
}

func TurnToColor(turn int) rl.Color {
	switch turn {
	case models.WHITE:
		return rl.White
	case models.BLACK:
		return rl.Black
	default:
		panic("invalid turn")
	}
}

func NewWindow(modeName string) (*Window, error) {
	var mode modes.Mode
	switch modeName {
	case "game":
		mode = modes.NewGame()
	case "evaluate":
		mode = modes.NewEvaluate()
	default:
		return nil, fmt.Errorf("invalid mode: %s", modeName)
	}

	return &Window{
		mode: mode,
	}, nil
}

func (w *Window) Run() {
	rl.SetTraceLogLevel(rl.LogError)

	rl.InitWindow(BoardWidthPx, BoardHeightPx, "Flippy")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	for !rl.WindowShouldClose() {
		w.handleEvents()
		w.mode.OnFrame()
		w.draw()
	}
}

func (w *Window) draw() {
	rl.BeginDrawing()

	backgroundColor := rl.NewColor(0, 128, 0, 255)
	rl.ClearBackground(backgroundColor)

	board := w.mode.GetBoard()
	uiOptions := w.mode.GetUIOptions()

	for index := range 64 {
		switch board.GetSquare(index) {
		case models.WHITE:
			w.drawDisc(index, TurnToColor(models.WHITE))
		case models.BLACK:
			w.drawDisc(index, TurnToColor(models.BLACK))
		case models.EMPTY:
			if board.IsValidMove(index) {
				color := TurnToColor(board.Turn())
				if eval, ok := uiOptions.Evaluations[index]; ok {
					w.drawEvaluation(index, eval, color)
					if eval == uiOptions.BestEvaluation {
						w.drawBestEvaluationIndicator(index, color)
					}
				} else {
					w.drawMoveIndicator(index, color)
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

func (w *Window) handleEvents() {
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mousePos := rl.GetMousePosition()
		mouseX := int(mousePos.X)
		mouseY := int(mousePos.Y)

		squareX := mouseX / SquareSize
		squareY := mouseY / SquareSize

		if squareX < 0 || squareX >= 8 || squareY < 0 || squareY >= 8 {
			w.mode.OnClick(rl.MouseLeftButton, mouseX, mouseY)
			return
		}

		index := squareX + squareY*8
		w.mode.OnMove(index)
		return
	}

	if rl.IsMouseButtonPressed(rl.MouseRightButton) {
		mousePos := rl.GetMousePosition()
		mouseX := int(mousePos.X)
		mouseY := int(mousePos.Y)
		w.mode.OnClick(rl.MouseRightButton, mouseX, mouseY)
	}

	if rl.IsKeyPressed(rl.KeyN) {
		w.mode.OnKeyPress(rl.KeyN)
	}
}

func (w *Window) drawEvaluation(index int, evaluation int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)

	fontSize := int32(30)

	text := strconv.Itoa(evaluation)
	textWidth := rl.MeasureText(text, fontSize)
	textX := centerX - textWidth/2
	textY := centerY - fontSize/2

	rl.DrawText(text, textX, textY, fontSize, color)
}

func (w *Window) drawBestEvaluationIndicator(index int, color rl.Color) {
	centerX, centerY := w.getSquareCenter(index)
	center := rl.Vector2{X: float32(centerX), Y: float32(centerY)}
	rl.DrawRing(center, DiscRadius-1, DiscRadius, 0, 360, 40, color)
}
