package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

// leaderboardCacheTTL is the maximum time a leaderboard result is served from
// cache before a fresh DB query is issued. The TTL is intentionally short (60s)
// because leaderboards are expected to update frequently during live matches.
// Explicit invalidation via InvalidateLeaderboard is the primary mechanism;
// TTL is the safety net for cases where invalidation is skipped.
const leaderboardCacheTTL = 60 * time.Second

func cacheKeyLeaderboard(quinielaID int) string {
	return fmt.Sprintf("leaderboard:%d", quinielaID)
}

// cachedRankingService wraps a Ranker with a read-through / write-invalidation
// cache. GetLeaderboard is served from the cache when available; callers that
// trigger score updates (e.g. the MatchFinished event handler) should call
// InvalidateLeaderboard to flush the cache for the affected quinielas.
//
// Cache failures are non-fatal: a miss or a Set error falls through to the
// inner Ranker so a Redis outage never makes the leaderboard unavailable.
type cachedRankingService struct {
	inner Ranker
	store cache.Store
	log   *zap.Logger
}

// NewCachedRankingService wraps ranker with leaderboard caching.
func NewCachedRankingService(ranker Ranker, store cache.Store, log *zap.Logger) *cachedRankingService {
	return &cachedRankingService{inner: ranker, store: store, log: log}
}

// GetLeaderboard returns the cached leaderboard for the given quiniela when
// available, or falls through to the inner Ranker and caches the result.
func (s *cachedRankingService) GetLeaderboard(ctx context.Context, quinielaID int) ([]*domain.LeaderboardEntry, error) {
	key := cacheKeyLeaderboard(quinielaID)
	var cached []*domain.LeaderboardEntry
	if err := s.store.Get(ctx, key, &cached); err == nil {
		return cached, nil
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		s.log.Warn("leaderboard cache get failed, falling through to db",
			zap.String("key", key), zap.Error(err))
	}

	entries, err := s.inner.GetLeaderboard(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		if setErr := s.store.Set(ctx, key, entries, leaderboardCacheTTL); setErr != nil {
			s.log.Warn("leaderboard cache set failed", zap.String("key", key), zap.Error(setErr))
		}
	}
	return entries, nil
}

// InvalidateLeaderboard removes the cached leaderboard for the given quiniela.
// Call this after a MatchFinished scoring run completes to ensure the next
// GetLeaderboard request reflects the new point totals.
func (s *cachedRankingService) InvalidateLeaderboard(ctx context.Context, quinielaID int) {
	if err := s.store.Delete(ctx, cacheKeyLeaderboard(quinielaID)); err != nil {
		s.log.Warn("leaderboard cache invalidation failed",
			zap.Int("quiniela_id", quinielaID), zap.Error(err))
	}
}
