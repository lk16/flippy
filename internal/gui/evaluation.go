package gui

import (
	"fmt"
	"log"
	"time"

	"github.com/lk16/flippy/api/internal/api"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/edax"
	"github.com/lk16/flippy/api/internal/othello"
)

var (
	// stopSearch will stop all searches when sent over searchChan.
	stopSearch othello.Board = othello.NewBoardEmpty()
)

type evaluateChanListener struct {
	// procs is a list of edax processes.
	procs []*edax.Process

	// edaxResultChan is a channel used to receive edax evaluations
	edaxResultChan chan edax.Result

	// searchChan is a channel used to start a new search and stop the previous one.
	searchChan chan othello.Board

	// evaluated is the currently evaluated board. An empty board indicates no evaluation was started yet.
	evaluated othello.Board

	// cache prevents recomputing same positions repeatedly.
	cache *edax.Cache

	// apiClient is the API client used to lookup positions.
	apiClient *api.Client

	// unsavedDBEvaluations is used to do periodic batch update posts to the api.
	unsavedDBEvaluations map[othello.NormalizedPosition]api.Evaluation
}

func newEvaluateChanListener(searchChan chan othello.Board, cache *edax.Cache) (*evaluateChanListener, error) {
	apiClient, err := api.NewClient(config.LoadLearnClientConfig())
	if err != nil {
		return nil, fmt.Errorf("error creating api client: %w", err)
	}

	return &evaluateChanListener{
		edaxResultChan:       make(chan edax.Result, 20),
		searchChan:           searchChan,
		procs:                []*edax.Process{},
		evaluated:            stopSearch,
		cache:                cache,
		apiClient:            apiClient,
		unsavedDBEvaluations: make(map[othello.NormalizedPosition]api.Evaluation, 20),
	}, nil
}

func (l *evaluateChanListener) Listen() {
	dbSaveTicker := time.NewTicker(time.Second)

	for {
		select {
		case update := <-l.edaxResultChan:
			l.handleEdaxResult(update)
		case board := <-l.searchChan:
			l.handleSearch(board)
		case <-dbSaveTicker.C:
			l.saveDBEvaluations()
		}
	}
}

func (l *evaluateChanListener) handleEdaxResult(result edax.Result) {
	if result.Err != nil {
		log.Printf("edax result error: %s", result.Err.Error())
		return
	}

	update := result.Result.Evaluation
	l.cache.Upsert(update)

	if update.Position.IsDBSavable() {
		// If the position is already present, we don't need to check if it's "better",
		// since we can assume that every engine update is "better".
		l.unsavedDBEvaluations[update.Position] = update
	}
}

func (l *evaluateChanListener) handleSearch(board othello.Board) {
	if board == stopSearch {
		l.killProcs()
		l.evaluated = stopSearch
		return
	}

	if board != l.evaluated {
		l.evaluated = board
		l.startSearch(board)
	}
}

// saveDBEvaluations batches recent evaluations to reduce load on the API.
func (l *evaluateChanListener) saveDBEvaluations() {
	if len(l.unsavedDBEvaluations) == 0 {
		return
	}

	// Use struct-level map as local var and reset it.
	// This needs to happen before creating go-routine to prevent race conditions.
	unsavedEvaluations := l.unsavedDBEvaluations
	l.unsavedDBEvaluations = make(map[othello.NormalizedPosition]api.Evaluation, 20)

	// Convert to slice.
	updates := make([]api.Evaluation, 0, len(unsavedEvaluations))
	for _, evaluation := range unsavedEvaluations {
		updates = append(updates, evaluation)
	}

	go func() {
		err := l.apiClient.SaveLearnedEvaluations(updates)
		if err != nil {
			log.Printf("error saving evaluation with api: %s", err.Error())
		}
	}()
}

func (l *evaluateChanListener) killProcs() {
	for _, proc := range l.procs {
		err := proc.Kill()
		if err != nil {
			log.Printf("error killing process: %s", err.Error())
		}
	}
	l.procs = []*edax.Process{}
}

func (l *evaluateChanListener) getSearchableChildren(board othello.Board) []othello.NormalizedPosition {
	normalizedChildren := board.GetNormalizedChildren()

	searchableChildren := []othello.NormalizedPosition{}
	for _, child := range normalizedChildren {
		if child.HasMoves() {
			searchableChildren = append(searchableChildren, child)
			continue
		}

		// Check if passing gives a searchable child.
		passed := child.Position().DoMove(othello.PassMove)
		if passed.HasMoves() {
			searchableChildren = append(searchableChildren, passed.Normalized())
		}

		// If neither player has moves, the child is not searchable.
	}

	return searchableChildren
}

func (l *evaluateChanListener) lookupAndUpdateCache(positions []othello.NormalizedPosition) {
	if len(positions) == 0 {
		return
	}

	evaluations, err := l.apiClient.LookupPositions(positions)
	if err != nil {
		log.Printf("error looking up positions: %s", err.Error())
		return
	}

	l.cache.BulkUpsert(evaluations)
}

func (l *evaluateChanListener) getLearnLevel(board othello.Board) int {
	childDiscCount := board.Position().CountDiscs() + 1

	if childDiscCount > config.MaxBookSavableDiscs {
		return 60
	}

	return api.GetLearnLevel(childDiscCount)
}

func (l *evaluateChanListener) startProcs(board othello.Board) {
	learnLevel := l.getLearnLevel(board)
	searchableChildren := l.getSearchableChildren(board)

	evaluationsToCompute := []othello.NormalizedPosition{}

	for _, child := range searchableChildren {
		if eval, ok := l.cache.Lookup(child); !ok || eval.Depth < learnLevel {
			evaluationsToCompute = append(evaluationsToCompute, child)
		}
	}

	var procs []*edax.Process
	for _, child := range evaluationsToCompute {
		job := api.Job{
			Position: child,
			Level:    learnLevel,
		}

		proc := edax.NewProcess(l.edaxResultChan)
		procs = append(procs, proc)
		go proc.DoJob(job, false)
	}

	l.procs = procs
}

func (l *evaluateChanListener) startSearch(board othello.Board) {
	l.killProcs()

	searchableChildren := l.getSearchableChildren(board)
	missingChildren := l.cache.GetMissing(searchableChildren)

	l.lookupAndUpdateCache(missingChildren)

	l.startProcs(board)

	l.cacheGrandchildren(board)
}

func (l *evaluateChanListener) cacheGrandchildren(board othello.Board) {
	children := board.GetChildren()

	if len(children) == 0 {
		children = board.DoMove(othello.PassMove).GetChildren()
	}

	var allGrandchildren []othello.NormalizedPosition
	seen := make(map[othello.NormalizedPosition]bool)

	for _, child := range children {
		for _, grandchild := range child.GetNormalizedChildren() {
			if seen[grandchild] {
				continue
			}
			seen[grandchild] = true
			allGrandchildren = append(allGrandchildren, grandchild)
		}
	}

	missingGrandchildren := l.cache.GetMissing(allGrandchildren)
	l.lookupAndUpdateCache(missingGrandchildren)
}
