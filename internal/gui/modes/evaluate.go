package modes

import (
	"fmt"
	"log" // nolint:depguard
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/models"
)

type Evaluate struct {
	gameMode *Game

	// computing is the board whose children are being computed.
	computing models.Board

	apiClient *book.APIClient

	evaluations      map[models.NormalizedPosition]models.Evaluation
	evaluationsMutex sync.Mutex
}

func NewEvaluate() *Evaluate {
	apiClient, err := book.NewAPIClient(config.LoadLearnClientConfig())
	if err != nil {
		log.Println("error creating api client", err)
		return nil
	}

	evaluate := &Evaluate{
		gameMode:    NewGame(),
		computing:   models.NewBoardEmpty(),
		apiClient:   apiClient,
		evaluations: make(map[models.NormalizedPosition]models.Evaluation),
	}

	return evaluate
}

var _ Mode = &Evaluate{}

func (e *Evaluate) GetBoard() models.Board {
	return e.gameMode.game.LastBoard()
}

func (e *Evaluate) OnMove(index int) {
	e.gameMode.OnMove(index)
}

func (e *Evaluate) OnClick(button rl.MouseButton, x, y int) {
	e.gameMode.OnClick(button, x, y)
}

func (e *Evaluate) OnKeyPress(key int) {
	e.gameMode.OnKeyPress(key)
}

func (e *Evaluate) OnFrame() {
	e.gameMode.OnFrame()
	err := e.ComputeLatestBoard()
	if err != nil {
		log.Printf("error computing latest board: %s", err.Error())
	}
}

func (e *Evaluate) GetUIOptions() *UIOPtions {
	evaluations := make(map[int]int)

	// TODO compute actual evaluations
	board := e.GetBoard()
	moves := board.Position().Moves()

	e.evaluationsMutex.Lock()
	for i := range 64 {
		if moves&(1<<i) != 0 {
			child := board.DoMove(i)
			if eval, ok := e.evaluations[child.Position().Normalized()]; ok {
				evaluations[i] = -eval.Score
			}
		}
	}
	e.evaluationsMutex.Unlock()

	return &UIOPtions{
		Evaluations: evaluations,
	}
}

func (e *Evaluate) ComputeLatestBoard() error {
	board := e.GetBoard()

	// If the board is the same as the one we're computing, do nothing.
	if e.computing == board {
		return nil
	}

	// TODO from this point on, run in a goroutine

	// TODO kill any running jobs

	normalizedChildren := board.GetNormalizedChildren()

	evaluations, err := e.apiClient.LookupPositions(normalizedChildren)
	if err != nil {
		return fmt.Errorf("error looking up positions: %w", err)
	}

	e.evaluationsMutex.Lock()
	for _, evaluation := range evaluations {
		if found, ok := e.evaluations[evaluation.Position]; ok {
			if found.Depth > evaluation.Depth {
				continue
			}
		}
		e.evaluations[evaluation.Position] = evaluation
	}
	e.evaluationsMutex.Unlock()

	// TODO start computation for all that don't have max depth

	e.computing = board
	return nil
}
