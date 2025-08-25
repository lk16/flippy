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

func (e *Evaluate) getUISquareEvaluation(move int) (int, bool) {
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()

	board := e.GetBoard()

	if !board.IsValidMove(move) {
		return 0, false
	}

	child := board.DoMove(move)

	if child.HasMoves() {
		if eval, ok := e.cache[child.Position().Normalized()]; ok {
			return -eval.Score, true
		} else {
			return 0, false
		}
	}

	passed := child.DoMove(models.PassMove)

	if passed.HasMoves() {
		if eval, ok := e.cache[passed.Position().Normalized()]; ok {
			return eval.Score, true
		} else {
			return 0, false
		}
	}

	return passed.GetFinalScore(), true
}

func (e *Evaluate) GetUIOptions() *UIOPtions {
	evaluations := make(map[int]int)

	for i := range 64 {
		if score, ok := e.getUISquareEvaluation(i); ok {
			evaluations[i] = score
		}
	}

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

func (e *Evaluate) getSearchableChildren(board models.Board) []models.NormalizedPosition {
	normalizedChildren := board.GetNormalizedChildren()

	searchableChildren := []models.NormalizedPosition{}
	for _, child := range normalizedChildren {
		if child.HasMoves() {
			searchableChildren = append(searchableChildren, child)
			continue
		}

		// Check if passing gives a searchable child.
		passed := child.Position().DoMove(models.PassMove)
		if passed.HasMoves() {
			searchableChildren = append(searchableChildren, passed.Normalized())
		}

		// If neither player has moves, the child is not searchable.
	}

	return searchableChildren
}

func (e *Evaluate) getMissingSearchableChildren(board models.Board) []models.NormalizedPosition {
	searchableChildren := e.getSearchableChildren(board)

	var missingChildren []models.NormalizedPosition

	e.cacheMutex.Lock()
	for _, child := range searchableChildren {
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

func (e *Evaluate) getLearnLevel(board models.Board) int {
	childDiscCount := board.Position().CountDiscs() + 1

	if childDiscCount > config.MaxBookSavableDiscs {
		return 60
	}

	return repository.GetLearnLevel(childDiscCount)
}

func (e *Evaluate) startProcs(board models.Board) {
	learnLevel := e.getLearnLevel(board)
	searchableChildren := e.getSearchableChildren(board)

	evaluationsToCompute := []models.NormalizedPosition{}

	e.cacheMutex.Lock()
	for _, child := range searchableChildren {
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

	missingChildren := e.getMissingSearchableChildren(board)
	e.lookupAndUpdateCache(missingChildren)

	e.startProcs(board)

	e.cacheGrandchildren(board)
}

func (e *Evaluate) cacheGrandchildren(board models.Board) {
	children := board.GetChildren()

	if len(children) == 0 {
		children = board.DoMove(models.PassMove).GetChildren()
	}

	var allGrandchildren []models.NormalizedPosition
	seen := make(map[models.NormalizedPosition]bool)

	for _, child := range children {
		for _, grandchild := range child.GetNormalizedChildren() {
			if seen[grandchild] {
				continue
			}
			seen[grandchild] = true
			allGrandchildren = append(allGrandchildren, grandchild)
		}
	}

	var missingGrandchildren []models.NormalizedPosition

	e.cacheMutex.Lock()
	for _, grandchild := range allGrandchildren {
		if _, ok := e.cache[grandchild]; !ok {
			missingGrandchildren = append(missingGrandchildren, grandchild)
		}
	}
	e.cacheMutex.Unlock()

	e.lookupAndUpdateCache(missingGrandchildren)
}
