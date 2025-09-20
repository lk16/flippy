package edax

import (
	"sync"

	"github.com/lk16/flippy/api/internal/api"
	"github.com/lk16/flippy/api/internal/othello"
)

// Cache implements a simple cache for othello evaluations.
type Cache struct {
	// data stores the underlying map
	data map[othello.NormalizedPosition]api.Evaluation

	// dataMutex protects data
	dataMutex sync.Mutex
}

// NewCache creates a new cache.
func NewCache() *Cache {
	return &Cache{
		data: make(map[othello.NormalizedPosition]api.Evaluation),
	}
}

// Upsert will add or update an entry in the cache if it adds more reliable information.
func (c *Cache) Upsert(evaluation api.Evaluation) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	c.upsertIfBetter(evaluation)
}

// BulkUpsert works like Upsert, but for multiple evaluations.
func (c *Cache) BulkUpsert(evaluations []api.Evaluation) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	for _, evaluation := range evaluations {
		c.upsertIfBetter(evaluation)
	}
}

// upsertIfBetter does an actual upsert. It assumes dataMutex is locked.
func (c *Cache) upsertIfBetter(evaluation api.Evaluation) {
	// use shorthand
	npos := evaluation.Position

	if !npos.HasMoves() {
		panic("cannot add normalized position without moves")
	}

	found, ok := c.data[npos]

	if !ok || evaluation.Depth > found.Depth {
		c.data[npos] = evaluation
	}
}

// Lookup looks up a position in the cache using a othello.NormalizedPosition.
func (c *Cache) Lookup(npos othello.NormalizedPosition) (api.Evaluation, bool) {
	if npos.HasMoves() {
		return c.lookup(npos)
	}

	passed := npos.Position().DoMove(othello.PassMove).Normalized()

	if passed.HasMoves() {
		return c.lookupPassed(npos, passed)
	}

	return c.getGameEnd(npos)
}

// lookup does a regular lookup.
func (c *Cache) lookup(npos othello.NormalizedPosition) (api.Evaluation, bool) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	eval, ok := c.data[npos]
	return eval, ok
}

// lookupPassed does a lookup for a position without moves.
func (c *Cache) lookupPassed(npos, passed othello.NormalizedPosition) (api.Evaluation, bool) {
	eval, ok := c.lookup(passed)
	if !ok {
		return api.Evaluation{}, false
	}

	// put correct position
	eval.Position = npos

	// prepend pass move
	bestMoves := []int{othello.PassMove}
	bestMoves = append(bestMoves, eval.BestMoves...)
	eval.BestMoves = bestMoves

	// flip score sign
	eval.Score = -eval.Score

	return eval, true
}

// getGameEnd creates a game end Evaluation from scratch.
func (c *Cache) getGameEnd(npos othello.NormalizedPosition) (api.Evaluation, bool) {
	empties := 64 - npos.CountDiscs()
	score := npos.Position().GetFinalScore()

	return api.Evaluation{
		Position:   npos,
		Depth:      empties,
		Level:      empties + (empties % 2),
		Confidence: 100,
		Score:      score,
		BestMoves:  []int{},
	}, true
}

// GetMissing returns a list of normalized positions that are not in the cache.
func (c *Cache) GetMissing(slice []othello.NormalizedPosition) []othello.NormalizedPosition {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	missing := make([]othello.NormalizedPosition, 0, len(slice))
	for _, npos := range slice {
		if _, ok := c.data[npos]; !ok {
			missing = append(missing, npos)
		}
	}

	return missing
}
