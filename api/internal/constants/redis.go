package constants

import "time"

const (
	PositionsKey        = "positions_to_learn"
	PositionsTTL        = 5 * time.Minute
	CacheRefreshLockKey = "positions_to_learn_refresh_lock"
	CacheRefreshLockTTL = 30 * time.Second
	PopRetryInterval    = 200 * time.Millisecond
	BookStatsKey        = "book_stats"
	// Queue size management
	InitialQueueSize = 500
	RefillThreshold  = 100
	JobRefreshSize   = 500
)
