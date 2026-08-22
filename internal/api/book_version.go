package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// bookVersionKey is the Redis counter bumped on every save that can change the minimax backfill;
// each replica polls it and rebuilds its cache on change.
const bookVersionKey = "book:version"

// bookVersionPollInterval is how often each replica polls bookVersionKey. Polling rather than
// pub/sub also covers a replica that started, or reconnected, while Redis was briefly unavailable.
const bookVersionPollInterval = 3 * time.Second

// cacheRebuildTimeout bounds a single minimax cache rebuild.
const cacheRebuildTimeout = 5 * time.Minute

// bumpBookVersion signals every replica, this one included, to rebuild its minimax cache.
func (s *Server) bumpBookVersion(ctx context.Context) error {
	if err := s.redis.Incr(ctx, bookVersionKey).Err(); err != nil {
		return fmt.Errorf("failed to bump book version: %w", err)
	}
	return nil
}

// RunCacheInvalidation builds the minimax cache, then rebuilds it whenever the book version
// changes, until ctx is canceled. Bumps arriving during a rebuild coalesce into one further
// rebuild on the next poll, not one each.
func (s *Server) RunCacheInvalidation(ctx context.Context) {
	// -1 forces the initial build even when the version key is missing (read as 0), and a failed
	// build leaves lastBuilt unchanged so the next tick retries.
	lastBuilt := int64(-1)
	lastBuilt = s.rebuildIfVersionChanged(ctx, lastBuilt)

	ticker := time.NewTicker(bookVersionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastBuilt = s.rebuildIfVersionChanged(ctx, lastBuilt)
		}
	}
}

// rebuildIfVersionChanged rebuilds the minimax cache if the book version moved past lastBuilt,
// returning the version the cache is now built at (lastBuilt unchanged on error, so the caller
// retries).
func (s *Server) rebuildIfVersionChanged(ctx context.Context, lastBuilt int64) int64 {
	version, err := s.redis.Get(ctx, bookVersionKey).Int64()
	if err == redis.Nil {
		version = 0
	} else if err != nil {
		log.Printf("failed to read book version: %v", err)
		return lastBuilt
	}

	if version == lastBuilt {
		return lastBuilt
	}

	rebuildCtx, cancel := context.WithTimeout(ctx, cacheRebuildTimeout)
	defer cancel()

	if err := s.cache.Rebuild(rebuildCtx); err != nil {
		log.Printf("failed to rebuild minimax cache: %v", err)
		return lastBuilt
	}

	log.Printf("minimax cache built: %d boards (book version %d)", s.cache.Len(), version)
	return version
}
