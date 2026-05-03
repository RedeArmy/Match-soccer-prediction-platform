package service

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

// Compile-time check: CachedRankingService must implement Ranker.
var _ Ranker = (*CachedRankingService)(nil)

// CachedRankingService wraps a Ranker with a read-through / write-invalidation
// cache. GetLeaderboard is served from the cache when available; callers that
// trigger score updates (e.g. the MatchFinished event handler) should call
// InvalidateLeaderboard to flush the cache for the affected quinielas.
//
// Cache failures are non-fatal: a miss or a Set error falls through to the
// inner Ranker so a Redis outage never makes the leaderboard unavailable.
//
// The active TTL is stored atomically so that an admin mutation of
// cache.leaderboard_ttl_seconds takes effect for all subsequent cache writes
// without requiring a process restart. Call UpdateTTL after reading the new
// value from SystemParamService.
type CachedRankingService struct {
	inner Ranker
	store cache.Store
	ttlNs atomic.Int64 // nanoseconds; read/written atomically
	log   *zap.Logger
}

// NewCachedRankingService wraps ranker with leaderboard caching.
// ttl is the initial cache duration; call UpdateTTL to change it at runtime.
func NewCachedRankingService(ranker Ranker, store cache.Store, ttl time.Duration, log *zap.Logger) *CachedRankingService {
	s := &CachedRankingService{inner: ranker, store: store, log: log}
	s.ttlNs.Store(ttl.Nanoseconds())
	return s
}

// UpdateTTL atomically replaces the TTL used for all subsequent cache writes.
// Existing cached entries are unaffected; call InvalidateAll to evict them.
func (s *CachedRankingService) UpdateTTL(d time.Duration) {
	s.ttlNs.Store(d.Nanoseconds())
}

func (s *CachedRankingService) effectiveTTL() time.Duration {
	return time.Duration(s.ttlNs.Load())
}

// GetLeaderboard returns the cached leaderboard for the given quiniela when
// available, or falls through to the inner Ranker and caches the result.
func (s *CachedRankingService) GetLeaderboard(ctx context.Context, quinielaID int) ([]*domain.LeaderboardEntry, error) {
	key := cacheKeyLeaderboard(quinielaID)
	if cached, ok := cacheGet[[]*domain.LeaderboardEntry](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	entries, err := s.inner.GetLeaderboard(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		cacheSet(ctx, s.store, key, entries, s.effectiveTTL(), s.log)
	}
	return entries, nil
}

// GetPhaseLeaderboard returns the cached phase leaderboard when available, or
// falls through to the inner Ranker and caches the result. Cache failures are
// non-fatal: a miss or a Set error falls through to the inner Ranker so a Redis
// outage never makes the leaderboard unavailable.
func (s *CachedRankingService) GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) ([]*domain.LeaderboardEntry, error) {
	key := cacheKeyPhaseLeaderboard(quinielaID, phase)
	if cached, ok := cacheGet[[]*domain.LeaderboardEntry](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	entries, err := s.inner.GetPhaseLeaderboard(ctx, quinielaID, phase)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		cacheSet(ctx, s.store, key, entries, s.effectiveTTL(), s.log)
	}
	return entries, nil
}

// InvalidateLeaderboard evicts all cached leaderboard entries for the given
// quiniela in a single Delete call. The set of keys is:
//
//   - leaderboard:{quinielaID}                          - overall standings
//   - leaderboard:{quinielaID}:phase:{phase}            - one key per phase
//
// All 8 keys (1 overall + 7 phases) are sent in a single Redis DEL command so
// the eviction is atomic from the cache's perspective. This prevents a race
// where the overall key is evicted but a phase key still serves stale data to
// a concurrent request that arrives between two individual Delete calls.
//
// Call this after a MatchFinished scoring run completes to ensure the next
// request for any variant of the leaderboard reflects the new point totals.
func (s *CachedRankingService) InvalidateLeaderboard(ctx context.Context, quinielaID int) {
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

// InvalidateAll evicts every leaderboard cache entry regardless of quiniela ID.
// It is invoked after a cache.leaderboard_ttl_seconds mutation so that the
// next request repopulates the cache with the updated TTL.
//
// Requires the underlying store to implement cache.PrefixFlusher (Redis does).
// On stores that do not support it this is a no-op; entries expire naturally.
func (s *CachedRankingService) InvalidateAll(ctx context.Context) {
	pf, ok := s.store.(cache.PrefixFlusher)
	if !ok {
		return
	}
	if err := pf.FlushByPrefix(ctx, "leaderboard:"); err != nil {
		s.log.Warn("leaderboard cache full flush failed", zap.Error(err))
	}
}
