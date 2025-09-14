package models

import (
	"sync"
)

// Cache implements a simple cache for othello evaluations.
type Cache struct {
	// data stores the underlying map
	data map[NormalizedPosition]Evaluation

	// dataMutex protects data
	dataMutex sync.Mutex
}

// NewCache creates a new cache.
func NewCache() *Cache {
	return &Cache{
		data: make(map[NormalizedPosition]Evaluation),
	}
}

// Upsert will add or update an entry in the cache if it adds more reliable information.
func (c *Cache) Upsert(evaluation Evaluation) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	c.upsertIfBetter(evaluation)
}

// BulkUpsert works like Upsert, but for multiple evaluations.
func (c *Cache) BulkUpsert(evaluations []Evaluation) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	for _, evaluation := range evaluations {
		c.upsertIfBetter(evaluation)
	}
}

// upsertIfBetter does an actual upsert. It assumes dataMutex is locked.
func (c *Cache) upsertIfBetter(evaluation Evaluation) {
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

// Lookup looks up a position in the cache using a NormalizedPosition.
func (c *Cache) Lookup(npos NormalizedPosition) (Evaluation, bool) {
	if npos.HasMoves() {
		return c.lookup(npos)
	}

	passed := npos.Position().DoMove(PassMove).Normalized()

	if passed.HasMoves() {
		return c.lookupPassed(npos, passed)
	}

	return c.getGameEnd(npos)
}

// lookup does a regular lookup.
func (c *Cache) lookup(npos NormalizedPosition) (Evaluation, bool) {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	eval, ok := c.data[npos]
	return eval, ok
}

// lookupPassed does a lookup for a position without moves.
func (c *Cache) lookupPassed(npos, passed NormalizedPosition) (Evaluation, bool) {
	eval, ok := c.lookup(passed)
	if !ok {
		return Evaluation{}, false
	}

	// put correct position
	eval.Position = npos

	// prepend pass move
	bestMoves := []int{PassMove}
	bestMoves = append(bestMoves, eval.BestMoves...)
	eval.BestMoves = bestMoves

	// flip score sign
	eval.Score = -eval.Score

	return eval, true
}

// getGameEnd creates a game end Evaluation from scratch.
func (c *Cache) getGameEnd(npos NormalizedPosition) (Evaluation, bool) {
	empties := 64 - npos.CountDiscs()
	score := npos.Position().GetFinalScore()

	return Evaluation{
		Position:   npos,
		Depth:      empties,
		Level:      empties + (empties % 2),
		Confidence: 100,
		Score:      score,
		BestMoves:  []int{},
	}, true
}

// GetMissing returns a list of normalized positions that are not in the cache.
func (c *Cache) GetMissing(slice []NormalizedPosition) []NormalizedPosition {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	missing := make([]NormalizedPosition, 0, len(slice))
	for _, npos := range slice {
		if _, ok := c.data[npos]; !ok {
			missing = append(missing, npos)
		}
	}

	return missing
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	c.dataMutex.Lock()
	defer c.dataMutex.Unlock()

	return len(c.data)
}
