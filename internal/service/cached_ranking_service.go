package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

// Compile-time check: cachedRankingService must implement Ranker.
var _ Ranker = (*cachedRankingService)(nil)

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
	ttl   time.Duration
	log   *zap.Logger
}

// NewCachedRankingService wraps ranker with leaderboard caching.
// ttl controls how long leaderboard results are cached; pass
// defaultLeaderboardCacheTTL (60s) when no system_param override is available.
func NewCachedRankingService(ranker Ranker, store cache.Store, ttl time.Duration, log *zap.Logger) *cachedRankingService {
	return &cachedRankingService{inner: ranker, store: store, ttl: ttl, log: log}
}

// GetLeaderboard returns the cached leaderboard for the given quiniela when
// available, or falls through to the inner Ranker and caches the result.
func (s *cachedRankingService) GetLeaderboard(ctx context.Context, quinielaID int) ([]*domain.LeaderboardEntry, error) {
	key := cacheKeyLeaderboard(quinielaID)
	if cached, ok := cacheGet[[]*domain.LeaderboardEntry](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	entries, err := s.inner.GetLeaderboard(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		cacheSet(ctx, s.store, key, entries, s.ttl, s.log)
	}
	return entries, nil
}

// GetPhaseLeaderboard returns the cached phase leaderboard when available, or
// falls through to the inner Ranker and caches the result. Cache failures are
// non-fatal: a miss or a Set error falls through to the inner Ranker so a Redis
// outage never makes the leaderboard unavailable.
func (s *cachedRankingService) GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) ([]*domain.LeaderboardEntry, error) {
	key := cacheKeyPhaseLeaderboard(quinielaID, phase)
	if cached, ok := cacheGet[[]*domain.LeaderboardEntry](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	entries, err := s.inner.GetPhaseLeaderboard(ctx, quinielaID, phase)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		cacheSet(ctx, s.store, key, entries, s.ttl, s.log)
	}
	return entries, nil
}

// InvalidateLeaderboard evicts all cached leaderboard entries for the given
// quiniela in a single Delete call. The set of keys is:
//
//   - leaderboard:{quinielaID}                          — overall standings
//   - leaderboard:{quinielaID}:phase:{phase}            — one key per phase
//
// All 8 keys (1 overall + 7 phases) are sent in a single Redis DEL command so
// the eviction is atomic from the cache's perspective. This prevents a race
// where the overall key is evicted but a phase key still serves stale data to
// a concurrent request that arrives between two individual Delete calls.
//
// Call this after a MatchFinished scoring run completes to ensure the next
// request for any variant of the leaderboard reflects the new point totals.
func (s *cachedRankingService) InvalidateLeaderboard(ctx context.Context, quinielaID int) {
	keys := make([]string, 0, 1+len(domain.AllMatchPhases))
	keys = append(keys, cacheKeyLeaderboard(quinielaID))
	for _, phase := range domain.AllMatchPhases {
		keys = append(keys, cacheKeyPhaseLeaderboard(quinielaID, phase))
	}
	if err := s.store.Delete(ctx, keys...); err != nil {
		s.log.Warn("leaderboard cache invalidation failed",
			zap.Int("quiniela_id", quinielaID), zap.Error(err))
	}
}
