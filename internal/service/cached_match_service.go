package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

// cachedMatchService wraps a MatchService with a read-through / write-
// invalidation cache layer. List operations are served from the cache when
// available; mutating operations (CreateMatch, UpdateResult, StartMatch)
// invalidate affected cache keys before delegating to the inner service.
//
// Cache failures are non-fatal: a miss or a Set error logs a warning and
// falls through to the inner service so that a Redis outage never degrades
// the API's correctness, only its latency.
type cachedMatchService struct {
	inner MatchService
	store cache.Store
	ttl   time.Duration
	log   *zap.Logger
}

// NewCachedMatchService wraps svc with cache-backed list operations.
// ttl controls how long list results are cached; pass defaultMatchCacheTTL
// (5m) when no system_param override is available. store must not be nil.
func NewCachedMatchService(svc MatchService, store cache.Store, ttl time.Duration, log *zap.Logger) MatchService {
	return &cachedMatchService{inner: svc, store: store, ttl: ttl, log: log}
}

// ListMatches returns all matches, using the cache when available.
func (s *cachedMatchService) ListMatches(ctx context.Context) ([]*domain.Match, error) {
	if cached, ok := cacheGet[[]*domain.Match](ctx, s.store, cacheKeyMatchesAll, s.log); ok {
		return cached, nil
	}
	matches, err := s.inner.ListMatches(ctx)
	if err != nil {
		return nil, err
	}
	cacheSet(ctx, s.store, cacheKeyMatchesAll, matches, s.ttl, s.log)
	return matches, nil
}

// ListMatchesByPhase returns matches for a specific tournament phase,
// using the cache when available.
func (s *cachedMatchService) ListMatchesByPhase(ctx context.Context, phase domain.MatchPhase) ([]*domain.Match, error) {
	key := cacheKeyMatchesByPhase(phase)
	if cached, ok := cacheGet[[]*domain.Match](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	matches, err := s.inner.ListMatchesByPhase(ctx, phase)
	if err != nil {
		return nil, err
	}
	cacheSet(ctx, s.store, key, matches, s.ttl, s.log)
	return matches, nil
}

// ListMatchesByStatus returns matches filtered by status, using the cache
// when available. The status cache is more volatile than phase because match
// status transitions happen throughout the tournament.
func (s *cachedMatchService) ListMatchesByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error) {
	key := cacheKeyMatchesByStatus(status)
	if cached, ok := cacheGet[[]*domain.Match](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	matches, err := s.inner.ListMatchesByStatus(ctx, status)
	if err != nil {
		return nil, err
	}
	cacheSet(ctx, s.store, key, matches, s.ttl, s.log)
	return matches, nil
}

// CreateMatch delegates to the inner service and invalidates all match list
// caches so the next read reflects the newly created fixture.
func (s *cachedMatchService) CreateMatch(ctx context.Context, match *domain.Match) error {
	if err := s.inner.CreateMatch(ctx, match); err != nil {
		return err
	}
	s.invalidateMatchLists(ctx, match.Phase, match.Status)
	return nil
}

// UpdateResult delegates to the inner service and invalidates relevant caches.
// The phase list and all status lists are invalidated because the match has
// transitioned from Live -> Finished; any cached Live or Finished list is stale.
func (s *cachedMatchService) UpdateResult(ctx context.Context, id int, homeScore, awayScore int) (*domain.Match, error) {
	m, err := s.inner.UpdateResult(ctx, id, homeScore, awayScore)
	if err != nil {
		return nil, err
	}
	s.invalidateMatchLists(ctx, m.Phase, m.Status)
	return m, nil
}

// StartMatch delegates to the inner service and invalidates caches affected
// by the Scheduled -> Live status transition.
func (s *cachedMatchService) StartMatch(ctx context.Context, id int) (*domain.Match, error) {
	m, err := s.inner.StartMatch(ctx, id)
	if err != nil {
		return nil, err
	}
	s.invalidateMatchLists(ctx, m.Phase, m.Status)
	return m, nil
}

// GetMatch delegates directly - single-entity reads are not cached because
// the cache benefit is small (one DB query) and the invalidation surface would
// grow with every match mutation.
func (s *cachedMatchService) GetMatch(ctx context.Context, id int) (*domain.Match, error) {
	return s.inner.GetMatch(ctx, id)
}

// invalidateMatchLists deletes all match list cache keys that could be stale
// after a mutation. The all-matches key is always deleted; phase and both
// status keys (the status before and after a transition) are also deleted.
func (s *cachedMatchService) invalidateMatchLists(ctx context.Context, phase domain.MatchPhase, status domain.MatchStatus) {
	keys := []string{
		cacheKeyMatchesAll,
		cacheKeyMatchesByPhase(phase),
		cacheKeyMatchesByStatus(status),
		// Always invalidate scheduled and live lists because any mutation can
		// affect them (e.g. a new match is scheduled, a live match finishes).
		cacheKeyMatchesByStatus(domain.MatchStatusScheduled),
		cacheKeyMatchesByStatus(domain.MatchStatusLive),
		cacheKeyMatchesByStatus(domain.MatchStatusFinished),
	}
	if err := s.store.Delete(ctx, keys...); err != nil {
		s.log.Warn("cache invalidation failed", zap.Error(err))
	}
}

var _ MatchService = (*cachedMatchService)(nil)
