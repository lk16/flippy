package book

import (
	"context"
	"fmt"
	"sync"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/othello"
)

// LeafDiscs is the disc count of the boards learned directly (see
// internal/db). Cache backfills evaluations for every board below it.
const LeafDiscs = 12

// Cache holds minimax-derived evaluations for boards with fewer than
// LeafDiscs discs, backfilled from the currently-learned LeafDiscs-disc
// evaluations in the DB. It's rebuilt from scratch on demand rather than
// updated incrementally: the tree below LeafDiscs discs is small enough
// that a full rebuild is cheap, and it keeps the logic simple.
//
// A Cache is safe for concurrent use.
type Cache struct {
	repo *db.Repository

	mu     sync.RWMutex
	values map[othello.Board]int
}

// NewCache returns an empty Cache backed by repo. Every Get misses until
// Rebuild has been called at least once.
func NewCache(repo *db.Repository) *Cache {
	return &Cache{repo: repo, values: make(map[othello.Board]int)}
}

// Rebuild recomputes the cache from the LeafDiscs-disc evaluations
// currently in the DB.
func (c *Cache) Rebuild(ctx context.Context) error {
	leaves, err := c.repo.EvaluatedBoards(ctx, LeafDiscs)
	if err != nil {
		return fmt.Errorf("failed to load leaf evaluations: %w", err)
	}

	leafScores := make(map[othello.Board]int, len(leaves))
	for _, be := range leaves {
		leafScores[be.Board.Board()] = be.Evaluation.Score
	}

	values := buildCache(othello.NewBoardStart(), LeafDiscs, leafScores)

	c.mu.Lock()
	c.values = values
	c.mu.Unlock()

	return nil
}

// Get returns the minimax score for board, from the perspective of the
// player to move, if it's covered by the cache. board doesn't need to
// already be normalized.
func (c *Cache) Get(board othello.Board) (int, bool) {
	normalized := board.Normalize().Board()

	c.mu.RLock()
	defer c.mu.RUnlock()

	score, ok := c.values[normalized]
	return score, ok
}

// Len returns the number of boards currently covered by the cache.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.values)
}
