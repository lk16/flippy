package modes

import (
	"log" // nolint:depguard
	"math"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

type Evaluate struct {
	// gameMode is used for storing the game state.
	gameMode *Game

	// apiClient is the API client used to lookup positions.
	apiClient *book.APIClient

	// cache is a map of normalized positions to evaluations.
	cache map[models.NormalizedPosition]models.Evaluation

	// cacheMutex is used to protect the cache map.
	cacheMutex sync.Mutex

	// procs is a list of edax processes.
	procs []*edax.Process

	// procsMutex is used to protect the procs list.
	procsMutex sync.Mutex

	// updateChan is a channel used to update the cache.
	updateChan chan models.Evaluation

	// isFirstFrame is used to know if this is the first frame of the game.
	isFirstFrame bool
}

func NewEvaluate() *Evaluate {
	apiClient, err := book.NewAPIClient(config.LoadLearnClientConfig())
	if err != nil {
		log.Println("error creating api client", err)
		return nil
	}

	return &Evaluate{
		gameMode:     NewGame(),
		apiClient:    apiClient,
		cache:        make(map[models.NormalizedPosition]models.Evaluation),
		procs:        []*edax.Process{},
		updateChan:   make(chan models.Evaluation, 20),
		isFirstFrame: true,
	}
}

var _ Mode = &Evaluate{}

func (e *Evaluate) GetBoard() models.Board {
	return e.gameMode.game.LastBoard()
}

func (e *Evaluate) OnMove(index int) {
	e.gameMode.OnMove(index)
	go e.OnBoardChange()
}

func (e *Evaluate) OnClick(button rl.MouseButton, x, y int) {
	e.gameMode.OnClick(button, x, y)
	go e.OnBoardChange()
}

func (e *Evaluate) OnKeyPress(key int) {
	e.gameMode.OnKeyPress(key)
	go e.OnBoardChange()
}

func (e *Evaluate) OnFrame() {
	e.gameMode.OnFrame()

	if e.isFirstFrame {
		go e.OnBoardChange()
		go e.handleUpdateChan()
		e.isFirstFrame = false
	}
}

func (e *Evaluate) handleUpdateChan() {
	for update := range e.updateChan {
		e.cacheMutex.Lock()
		cachedEval, cacheFound := e.cache[update.Position]

		if !cacheFound || update.Depth > cachedEval.Depth {
			log.Printf(
				"updating cache for pos=%s depth=%2d score=%d",
				update.Position.String(),
				update.Depth,
				update.Score,
			)
			e.cache[update.Position] = update
		}
		e.cacheMutex.Unlock()
	}
}

func (e *Evaluate) GetUIOptions() *UIOPtions {
	evaluations := make(map[int]int)

	board := e.GetBoard()
	moves := board.Position().Moves()

	e.cacheMutex.Lock()
	for i := range 64 {
		if moves&(1<<i) != 0 {
			child := board.DoMove(i)
			if eval, ok := e.cache[child.Position().Normalized()]; ok {
				evaluations[i] = -eval.Score
			}
		}
	}
	e.cacheMutex.Unlock()

	bestEvaluation := math.MinInt
	for _, eval := range evaluations {
		if eval > bestEvaluation {
			bestEvaluation = eval
		}
	}

	return &UIOPtions{
		Evaluations:    evaluations,
		BestEvaluation: bestEvaluation,
	}
}

func (e *Evaluate) killProcs() {
	e.procsMutex.Lock()
	for _, proc := range e.procs {
		err := proc.Kill()
		if err != nil {
			log.Printf("error killing process: %s", err.Error())
		}
	}
	e.procs = []*edax.Process{}
	e.procsMutex.Unlock()
}

func (e *Evaluate) getMissingChildren(board models.Board) []models.NormalizedPosition {
	normalizedChildren := board.GetNormalizedChildren()

	// TODO handle children without moves

	var missingChildren []models.NormalizedPosition
	e.cacheMutex.Lock()
	for _, child := range normalizedChildren {
		if _, ok := e.cache[child]; !ok {
			missingChildren = append(missingChildren, child)
		}
	}
	e.cacheMutex.Unlock()

	return missingChildren
}

func (e *Evaluate) lookupAndUpdateCache(positions []models.NormalizedPosition) {
	if len(positions) == 0 {
		return
	}

	evaluations, err := e.apiClient.LookupPositions(positions)
	if err != nil {
		log.Printf("error looking up positions: %s", err.Error())
		return
	}

	e.cacheMutex.Lock()
	for _, evaluation := range evaluations {
		e.cache[evaluation.Position] = evaluation
	}
	e.cacheMutex.Unlock()
}

func (e *Evaluate) startProcs(board models.Board) {
	learnLevel := repository.GetLearnLevel(board.Position().CountDiscs() + 1)
	normalizedChildren := board.GetNormalizedChildren()

	evaluationsToCompute := []models.NormalizedPosition{}
	e.cacheMutex.Lock()
	for _, child := range normalizedChildren {
		if eval, ok := e.cache[child]; !ok || eval.Depth < learnLevel {
			evaluationsToCompute = append(evaluationsToCompute, child)
		}
	}
	e.cacheMutex.Unlock()

	var procs []*edax.Process
	for _, child := range evaluationsToCompute {
		job := models.Job{
			Position: child,
			Level:    learnLevel,
		}

		proc := edax.NewProcess()
		proc.SetEvaluationsChannel(e.updateChan)
		procs = append(procs, proc)
		go func() { _, _ = proc.DoJob(job) }()
	}

	e.procsMutex.Lock()
	e.procs = procs
	e.procsMutex.Unlock()
}

func (e *Evaluate) OnBoardChange() {
	board := e.GetBoard()

	e.killProcs()

	missingChildren := e.getMissingChildren(board)
	e.lookupAndUpdateCache(missingChildren)

	e.startProcs(board)
}
