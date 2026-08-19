package book

import (
	"context"
	"fmt"
	"sync"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// LeafDiscs is the disc count of the positions learned directly; Cache backfills everything below it.
const LeafDiscs = 12

// MaxSavableDiscs is the highest disc count worth a row in the boards table.
const MaxSavableDiscs = 30

// Cache holds minimax-derived evaluations for positions below LeafDiscs, rebuilt from scratch on
// demand rather than updated incrementally. Safe for concurrent use.
type Cache struct {
	repo *db.Repository

	mu     sync.RWMutex
	values map[othello.Position]int
}

// NewCache returns an empty Cache backed by repo; every Get misses until Rebuild is called.
func NewCache(repo *db.Repository) *Cache {
	return &Cache{repo: repo, values: make(map[othello.Position]int)}
}

// Rebuild recomputes the cache from the LeafDiscs-disc evaluations currently in the DB.
func (c *Cache) Rebuild(ctx context.Context) error {
	leaves, err := c.repo.EvaluatedPositions(ctx, LeafDiscs)
	if err != nil {
		return fmt.Errorf("failed to load leaf evaluations: %w", err)
	}

	leafScores := make(map[othello.Position]int, len(leaves))
	for _, be := range leaves {
		leafScores[be.Position.Position()] = be.Evaluation.Score
	}

	values := buildCache(othello.NewStartPosition(), LeafDiscs, leafScores)

	c.mu.Lock()
	c.values = values
	c.mu.Unlock()

	return nil
}

// Get returns position's minimax score, from the perspective of the player to move, if the cache
// covers it.
func (c *Cache) Get(position othello.Position) (int, bool) {
	normalized := position.Normalize().Position()

	c.mu.RLock()
	defer c.mu.RUnlock()

	score, ok := c.values[normalized]
	return score, ok
}

// Len returns the number of positions currently covered by the cache.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.values)
}
