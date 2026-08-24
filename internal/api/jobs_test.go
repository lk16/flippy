package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lk16/flippy/internal/book"
	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/othello"
)

func TestServer_ClaimJob_PicksLowestDiscCountThenLevel(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position12 := testPosition(t, 12)
	position13s := testDistinctPositions(t, 13, 2)

	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position13s[0], position13s[1], position12}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position12, job.Position)
	require.Equal(t, UnlearnedLevel(12), job.Level)
}

func TestServer_ClaimJob_SkipsAlreadyClaimedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	positions := testDistinctPositions(t, 12, 2)
	require.NoError(t, s.repo.AddPositions(ctx, positions))

	job1, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	job2, ok, err := s.claimJob(ctx, "worker-2")
	require.NoError(t, err)
	require.True(t, ok)

	require.NotEqual(t, job1.Position, job2.Position)
}

func TestServer_ClaimJob_SkipsFullyLearnedBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// Covers the starvation bug ListLearnable's level cutoff prevents: fully learned leaves sort
// ahead of deeper unlearned positions and would otherwise fill the whole candidate batch.
func TestServer_ClaimJob_LeafBoardsDoNotStarveDeeperCandidates(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	leafBoards := testDistinctPositions(t, 12, jobCandidateBatch+1)
	require.NoError(t, s.repo.AddPositions(ctx, leafBoards))
	for _, position := range leafBoards {
		require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{
			Level: TargetLevel(12), Score: 0,
		}))
	}

	position13 := testPosition(t, 13)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position13}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position13, job.Position)
	require.Equal(t, UnlearnedLevel(13), job.Level)
}

// TestServer_ClaimJob_BuffersTheRestOfTheRefill covers the point of the buffer: one claim's DB scan
// leaves candidates ready for the claims after it.
func TestServer_ClaimJob_BuffersTheRestOfTheRefill(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	positions := testDistinctPositions(t, 12, 3)
	require.NoError(t, s.repo.AddPositions(ctx, positions))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)

	buffered, err := s.redis.LLen(ctx, jobBufferKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(len(positions)-1), buffered)
}

// TestServer_ClaimJob_WrapsSweepAfterAClaimExpires covers the recycling the cursor has to preserve:
// the sweep has already passed a position whose claim later expires, so only wrapping re-offers it.
func TestServer_ClaimJob_WrapsSweepAfterAClaimExpires(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position, job.Position)

	// Stand in for the claim TTL running out on a worker that died mid-job.
	require.NoError(t, s.redis.Del(ctx, claimKey(position.String())).Err())

	job, ok, err = s.claimJob(ctx, "worker-2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position, job.Position)
}

// TestServer_ClaimJob_SkipsCandidateLearnedSinceItWasBuffered covers the staleness a buffer
// introduces: a position can reach its target level between the refill that buffered it and the pop
// that hands it out.
func TestServer_ClaimJob_SkipsCandidateLearnedSinceItWasBuffered(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.redis.RPush(ctx, jobBufferKey, position.String()).Err())
	require.NoError(t, s.repo.SaveEvaluation(ctx, position, db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestServer_ClaimJob_SkipsCandidateWithoutARow covers a buffered position deleted from the book
// before it was handed out.
func TestServer_ClaimJob_SkipsCandidateWithoutARow(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.redis.RPush(ctx, jobBufferKey, position.String()).Err())

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestServer_ClaimJob_DoesNotRefillWhileAnotherReplicaHoldsTheLock covers the lock's purpose: only
// one replica scans the DB per refill, and the rest wait rather than piling on.
func TestServer_ClaimJob_DoesNotRefillWhileAnotherReplicaHoldsTheLock(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position := testPosition(t, 12)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position}))
	require.NoError(t, s.redis.Set(ctx, jobRefillLockKey, "other-replica", jobRefillLockTTL).Err())

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)

	// The lock is the other replica's to release.
	holder, err := s.redis.Get(ctx, jobRefillLockKey).Result()
	require.NoError(t, err)
	require.Equal(t, "other-replica", holder)
}

// TestServer_ClaimJob_SkipsOutOfRangeDiscCounts covers both ends of the savable range end to end:
// AddPositions drops such a position rather than storing it, so no job is ever handed out for one.
func TestServer_ClaimJob_SkipsOutOfRangeDiscCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	outOfRange := []othello.NormalizedPosition{
		testPosition(t, book.LeafDiscs-1),
		testPosition(t, book.MaxSavableDiscs+5),
	}
	require.NoError(t, s.repo.AddPositions(ctx, outOfRange))

	for _, position := range outOfRange {
		_, err := s.repo.GetPosition(ctx, position.Position())
		require.ErrorIs(t, err, db.ErrPositionNotFound)
	}

	_, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestServer_ClaimJob_NoBoardsAvailable(t *testing.T) {
	s := testServer(t)

	_, ok, err := s.claimJob(context.Background(), "worker-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestServer_ClaimJob_StaleFloorSkipsNewlyAddedLowerBoards documents the accepted trade-off of
// deriving the floor from the periodically rebuilt book_stats hash: positions added below the floor
// after the last rebuild are invisible to claimJob until the next rebuild.
func TestServer_ClaimJob_StaleFloorSkipsNewlyAddedLowerBoards(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	position12s := testDistinctPositions(t, 12, 2)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position12s[0]}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, position12s[0], db.Evaluation{
		Level: TargetLevel(12), Score: 0,
	}))
	position13 := testPosition(t, 13)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position13}))

	// The rebuilt floor is 13: every 12-disc position known to the hash is fully learned.
	require.NoError(t, s.rebuildBookStats(ctx))

	// A 12-disc position added after the rebuild stays invisible until the next one.
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{position12s[1]}))

	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, position13, job.Position)
}

// TestTargetLevel covers the tier boundaries and the parity alignment on top of them: a
// depth-limited target at an odd disc count is one level above its tier's.
func TestTargetLevel(t *testing.T) {
	require.Equal(t, 40, TargetLevel(12))
	require.Equal(t, 41, TargetLevel(13))
	require.Equal(t, 36, TargetLevel(14))
	require.Equal(t, 37, TargetLevel(15))
	require.Equal(t, 36, TargetLevel(16))
	require.Equal(t, 35, TargetLevel(17))
	require.Equal(t, 34, TargetLevel(20))
	require.Equal(t, 33, TargetLevel(21))
	require.Equal(t, 32, TargetLevel(30))
	require.Equal(t, 32, TargetLevel(64))
}

// TestTargetLevel_AlternatesDepthParity is the point of the alignment: no two adjacent plies may be
// searched at the same depth parity, or a line of best moves alternates by ~1.6 discs. From 25 discs
// up the search runs the game out and its depth is the empty count, which alternates on its own.
func TestTargetLevel_AlternatesDepthParity(t *testing.T) {
	for discCount := book.LeafDiscs; discCount <= book.MaxSavableDiscs; discCount++ {
		depth, _ := edax.SearchParams(discCount, TargetLevel(discCount))
		require.Equal(t, discCount%2, depth%2, "disc count %d searches %d ply", discCount, depth)
	}
}

// TestTargetLevel_ConfidenceUnchangedByAlignment guards against the alignment silently buying a
// different selectivity: raising the level by one must only add a ply.
func TestTargetLevel_ConfidenceUnchangedByAlignment(t *testing.T) {
	for discCount := book.LeafDiscs; discCount <= book.MaxSavableDiscs; discCount++ {
		_, tierConfidence := edax.SearchParams(discCount, tierLevel(discCount))
		_, confidence := edax.SearchParams(discCount, TargetLevel(discCount))
		require.Equal(t, tierConfidence, confidence, "disc count %d", discCount)
	}
}

// TestEffectiveTargetLevel_AlignsOnTheRealDiscCount covers the one place the two lookups differ:
// past MaxSavableDiscs the tier is capped, but the parity rule still uses the position's own count.
func TestEffectiveTargetLevel_AlignsOnTheRealDiscCount(t *testing.T) {
	for discCount := range 65 {
		want := edax.AlignLevel(discCount, tierLevel(min(discCount, book.MaxSavableDiscs)))
		require.Equal(t, want, EffectiveTargetLevel(discCount), "disc count %d", discCount)
	}
	require.Equal(t, TargetLevel(book.MaxSavableDiscs), EffectiveTargetLevel(book.MaxSavableDiscs+2))
}

// TestTargetLevelTiers_MatchEffectiveTargetLevel guards the contract handleLevelConfig relies on:
// the tiers plus the parity bumps served to the frontend must reproduce EffectiveTargetLevel for
// every disc count, so the frontend's target for a position is exactly the one
// handleAnalyzeRequest clamps its requests to.
func TestTargetLevelTiers_MatchEffectiveTargetLevel(t *testing.T) {
	tiers := TargetLevelTiers()
	require.NotEmpty(t, tiers)
	require.Equal(t, 64, tiers[len(tiers)-1].MaxDiscs, "last tier must cover a full board")

	bumped := make(map[int]bool)
	for _, discCount := range ParityBumpDiscs() {
		bumped[discCount] = true
	}

	for discCount := range 65 {
		var want int
		for _, tier := range tiers {
			if min(discCount, book.MaxSavableDiscs) <= tier.MaxDiscs {
				want = tier.Level
				break
			}
		}
		if bumped[discCount] {
			want++
		}
		require.Equal(t, want, EffectiveTargetLevel(discCount), "disc count %d", discCount)
	}
}

// TestIsBookQuality covers the level floor handleSubmitJobResult applies to every submission: only
// a search at least as deep as the shallowest job tier -- or one that already ran the game out --
// may reach the DB.
func TestIsBookQuality(t *testing.T) {
	tests := []struct {
		name      string
		discCount int
		level     int
		want      bool
	}{
		{"below the unlearned tier", 14, UnlearnedLevel(14) - 1, false},
		{"at the unlearned tier", 14, UnlearnedLevel(14), true},
		{"between the tiers", 14, TargetLevel(14) - 2, true},
		{"at target", 14, TargetLevel(14), true},
		{"above target", 14, TargetLevel(14) + 2, true},
		{"at target, deepest tier", 30, TargetLevel(30), true},
		// 52 discs is past MaxSavableDiscs, so the disc-count check keeps it out of the DB anyway,
		// but it is the shape the IsFinal clause exists for: a shallow search that is still the
		// game-theoretic result, which no deeper level could improve on.
		{"below the unlearned tier but final", 52, 12, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isBookQuality(tt.discCount, tt.level))
		})
	}
}

// TestTargetLevelTiers_ReturnsACopy makes sure a caller cannot rewrite the table TargetLevel reads.
func TestTargetLevelTiers_ReturnsACopy(t *testing.T) {
	TargetLevelTiers()[0].Level = 1
	require.Equal(t, 40, TargetLevel(4))
}

// TestServer_ClaimJob_LevelPerTier covers what separates the second job tier from the third: an
// unlearned row is searched at UnlearnedLevel, a row that already has a score at its target.
func TestServer_ClaimJob_LevelPerTier(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()

	unlearned := testPosition(t, 14)
	belowTarget := testPosition(t, 16)
	require.NoError(t, s.repo.AddPositions(ctx, []othello.NormalizedPosition{unlearned, belowTarget}))
	require.NoError(t, s.repo.SaveEvaluation(ctx, belowTarget, db.Evaluation{
		Level: UnlearnedLevel(16), Score: 0,
	}))

	// Unlearned first, whatever the disc counts (see db.ListLearnable's ordering).
	job, ok, err := s.claimJob(ctx, "worker-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, unlearned, job.Position)
	require.Equal(t, UnlearnedLevel(14), job.Level)

	job, ok, err = s.claimJob(ctx, "worker-2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, belowTarget, job.Position)
	require.Equal(t, TargetLevel(16), job.Level)
}
