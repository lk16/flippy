package gui

import (
	"fmt"
	"log"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/othello"
)

type Controller struct {
	// game contains the move history
	game *othello.Game

	// cache prevents recomputing same positions repeatedly
	cache *edax.Cache

	// searchChan is a channel used to start a new search and stop the previous one.
	searchChan chan othello.Board

	// evaluationEnabled indicates if we're evaluating positions
	evaluationEnabled bool

	// showSearchDepth indicates if we show the search level next to evaluations
	showSearchDepth bool
}

func NewWindow(start othello.Board) (*Controller, error) {
	searchChan := make(chan othello.Board, 20)
	cache := edax.NewCache()

	listener, err := newEvaluateChanListener(searchChan, cache)
	if err != nil {
		return nil, fmt.Errorf("could not create channal listener: %w", err)
	}

	go listener.Listen()

	w := &Controller{
		game:              othello.NewGameWithStart(start),
		searchChan:        searchChan,
		cache:             cache,
		evaluationEnabled: true,
		showSearchDepth:   false,
	}

	w.OnBoardChange()

	return w, nil
}

func (c *Controller) Run() {
	rl.SetTraceLogLevel(rl.LogError)

	rl.InitWindow(BoardWidthPx, BoardHeightPx, "Flippy")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	windowDrawer := newWindowDrawer(c)

	for !rl.WindowShouldClose() {
		c.handleEvents()
		windowDrawer.draw()
	}
}

func (c *Controller) handleEvents() {
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mousePos := rl.GetMousePosition()
		mouseX := int(mousePos.X)
		mouseY := int(mousePos.Y)

		squareX := mouseX / SquareSize
		squareY := mouseY / SquareSize

		if squareX < 0 || squareX >= 8 || squareY < 0 || squareY >= 8 {
			c.OnClick(rl.MouseLeftButton, mouseX, mouseY)
			return
		}

		index := squareX + squareY*8
		if c.game.GetBoard().IsValidMove(index) {
			c.OnMove(index)
		}
		return
	}

	if rl.IsMouseButtonPressed(rl.MouseRightButton) {
		mousePos := rl.GetMousePosition()
		mouseX := int(mousePos.X)
		mouseY := int(mousePos.Y)
		c.OnClick(rl.MouseRightButton, mouseX, mouseY)
	}

	for {
		key := rl.GetKeyPressed()
		if key == rl.KeyNull {
			break
		}

		c.OnKeyPress(key)
	}
}

func (c *Controller) GetBoard() othello.Board {
	return c.game.GetBoard()
}

func (c *Controller) OnMove(index int) {
	if err := c.game.PushMove(index); err != nil {
		panic(err.Error())
	}

	c.OnBoardChange()
}

func (c *Controller) OnClick(button rl.MouseButton, _, _ int) {
	if button == rl.MouseRightButton {
		c.game.PopMove()
		return
	}

	c.OnBoardChange()
}

func (c *Controller) OnKeyPress(key int32) {
	// Print current board.
	if key == rl.KeyD {
		log.Printf("current board = %s", c.GetBoard().String())
	}

	// Restart game
	if key == rl.KeyN {
		c.game = othello.NewGame()
	}

	// Toggle showing and computing evaluations
	if key == rl.KeyE {
		// toggle setting
		c.evaluationEnabled = !c.evaluationEnabled
		if c.evaluationEnabled {
			c.searchChan <- c.GetBoard()
		} else {
			c.searchChan <- stopSearch
		}
	}

	// Toggle showing search depth
	if key == rl.KeySpace {
		c.showSearchDepth = !c.showSearchDepth
	}

	c.OnBoardChange()
}

func (c *Controller) GetDrawArgs() *DrawArgs {
	board := c.GetBoard()

	return &DrawArgs{
		Board:             board,
		SquareEvaluations: c.getSquareEvaluationMap(board),
		ShowSearchDepth:   c.showSearchDepth,
		ShowEvaluations:   c.evaluationEnabled,
	}
}

func (c *Controller) getSquareEvaluationMap(board othello.Board) map[int]*MoveEvaluation {
	evals := make(map[int]*MoveEvaluation)

	for i := range 64 {
		if !board.IsValidMove(i) {
			continue
		}

		child := board.DoMove(i)
		normalized := child.Position().Normalized()

		if eval, ok := c.cache.Lookup(normalized); ok {
			evals[i] = &MoveEvaluation{
				Score:  -eval.Score,
				Depth:  eval.Depth,
				IsBest: false, // Updated below
			}
		}
	}

	bestScore := math.MinInt
	for _, eval := range evals {
		if eval.Score > bestScore {
			bestScore = eval.Score
		}
	}

	for _, squareEval := range evals {
		if squareEval.Score == bestScore {
			squareEval.IsBest = true
		}
	}

	return evals
}

func (c *Controller) OnBoardChange() {
	if c.evaluationEnabled {
		c.searchChan <- c.GetBoard()
	}
}
