package service

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/repository"
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
//
// Snapshot fallback (optional, enabled via WithSnapshotFallback):
// On a cold-cache miss for GetLeaderboard, the service tries the latest DB
// snapshot before falling back to a full live computation. For groups with
// many predictions (>10k rows) this avoids an expensive GROUP BY aggregation
// on every Redis restart or cache eviction, replacing it with a fast indexed
// snapshot lookup plus a batch user-name enrichment.
// Phase leaderboards always compute live (no per-phase snapshots).
type CachedRankingService struct {
	inner      Ranker
	store      cache.Store
	ttlNs      atomic.Int64 // nanoseconds; read/written atomically
	log        *zap.Logger
	snapRepo   repository.LeaderboardSnapshotRepository // nil when snapshot fallback is disabled
	userRepo   repository.UserRepository                // nil when snapshot fallback is disabled
	memberRepo repository.GroupMembershipRepository     // nil when snapshot fallback is disabled
}

// CachedRankingOption is a functional option for CachedRankingService.
type CachedRankingOption func(*CachedRankingService)

// WithSnapshotFallback enables the snapshot-backed cold-cache path for
// GetLeaderboard. When the Redis cache misses and a snapshot is available in
// the DB, the result is built from the snapshot + a user batch-lookup (one
// query) + active-paid count — avoiding the expensive GROUP BY aggregation
// over raw predictions.
//
// snapRepo provides the latest snapshot per quiniela. userRepo enriches
// snapshot entries with user display names. memberRepo provides the
// authoritative active-paid count for prize metadata.
func WithSnapshotFallback(
	snapRepo repository.LeaderboardSnapshotRepository,
	userRepo repository.UserRepository,
	memberRepo repository.GroupMembershipRepository,
) CachedRankingOption {
	return func(s *CachedRankingService) {
		s.snapRepo = snapRepo
		s.userRepo = userRepo
		s.memberRepo = memberRepo
	}
}

// NewCachedRankingService wraps ranker with leaderboard caching.
// ttl is the initial cache duration; call UpdateTTL to change it at runtime.
func NewCachedRankingService(ranker Ranker, store cache.Store, ttl time.Duration, log *zap.Logger, opts ...CachedRankingOption) *CachedRankingService {
	s := &CachedRankingService{inner: ranker, store: store, log: log}
	s.ttlNs.Store(ttl.Nanoseconds())
	for _, o := range opts {
		o(s)
	}
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

// GetLeaderboard returns the cached LeaderboardResult for the given quiniela
// when available. On a cache miss the read cascade is:
//
//  1. Redis cache (fast, sub-millisecond on hit)
//  2. Latest DB snapshot (WithSnapshotFallback only) — avoids a full GROUP BY
//     aggregation over raw predictions when the cache is cold (e.g. after a
//     Redis restart). Uses snapshot entries + batch user-name enrichment.
//  3. Full live computation via the inner Ranker (always available as fallback)
//
// The full LeaderboardResult (including prize metadata) is cached so that
// ActivePaidMembers, WinnerCount, and EligibleForPrizes are consistent with
// the cached entries and do not require a separate DB round-trip on cache hit.
func (s *CachedRankingService) GetLeaderboard(ctx context.Context, quinielaID int) (*LeaderboardResult, error) {
	key := cacheKeyLeaderboard(quinielaID)
	if cached, ok := cacheGet[*LeaderboardResult](ctx, s.store, key, s.log); ok {
		return cached, nil
	}

	// Snapshot fallback: cheaper than live computation for large groups.
	if s.snapRepo != nil {
		if result, ok, err := s.resultFromSnapshot(ctx, quinielaID); err == nil && ok {
			cacheSet(ctx, s.store, key, result, s.effectiveTTL(), s.log)
			return result, nil
		}
		// Snapshot miss or enrichment failure: fall through to full computation.
	}

	result, err := s.inner.GetLeaderboard(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if result != nil && len(result.Entries) > 0 {
		cacheSet(ctx, s.store, key, result, s.effectiveTTL(), s.log)
	}
	return result, nil
}

// resultFromSnapshot attempts to build a LeaderboardResult from the latest DB
// snapshot for quinielaID without executing the expensive GROUP BY aggregation
// over raw predictions.
//
// It executes two DB queries:
//  1. LeaderboardSnapshotRepository.GetLatest — indexed lookup (fast).
//  2. UserRepository.ListByIDs — batch fetch of display names.
//  3. GroupMembershipRepository.CountActivePaid — authoritative prize count.
//
// Returns (result, true, nil) on success, (nil, false, nil) when no snapshot
// exists yet, and (nil, false, err) when the snapshot lookup or enrichment fails.
// The caller falls through to full live computation on any non-success case.
func (s *CachedRankingService) resultFromSnapshot(ctx context.Context, quinielaID int) (*LeaderboardResult, bool, error) {
	snap, err := s.snapRepo.GetLatest(ctx, quinielaID)
	if err != nil {
		s.log.Warn("snapshot fallback: GetLatest failed, falling back to live computation",
			zap.Int("quiniela_id", quinielaID),
			zap.Error(err),
		)
		return nil, false, err
	}
	if snap == nil || len(snap.Entries) == 0 {
		return nil, false, nil // no snapshot yet — caller does full computation
	}

	// Batch-fetch user display names for all entries in the snapshot.
	userIDs := make([]int, 0, len(snap.Entries))
	for _, e := range snap.Entries {
		userIDs = append(userIDs, e.UserID)
	}
	users, err := s.userRepo.ListByIDs(ctx, userIDs)
	if err != nil {
		s.log.Warn("snapshot fallback: user enrichment failed, falling back to live computation",
			zap.Int("quiniela_id", quinielaID),
			zap.Error(err),
		)
		return nil, false, err
	}
	userByID := make(map[int]*domain.User, len(users))
	for _, u := range users {
		userByID[u.ID] = u
	}

	// Authoritative active-paid count for prize metadata.
	activePaid, err := s.memberRepo.CountActivePaid(ctx, quinielaID)
	if err != nil {
		s.log.Warn("snapshot fallback: CountActivePaid failed, falling back to live computation",
			zap.Int("quiniela_id", quinielaID),
			zap.Error(err),
		)
		return nil, false, err
	}

	entries := make([]*domain.LeaderboardEntry, 0, len(snap.Entries))
	for _, se := range snap.Entries {
		u, ok := userByID[se.UserID]
		if !ok {
			// Soft-deleted user — skip (matches live computation behaviour).
			continue
		}
		entries = append(entries, &domain.LeaderboardEntry{
			User:        u,
			Rank:        se.Rank,
			TotalPoints: se.TotalPoints,
			PrizeWinner: se.PrizeWinner,
		})
	}

	result := &LeaderboardResult{
		Entries:           entries,
		ActivePaidMembers: activePaid,
		WinnerCount:       domain.WinnerCount(activePaid),
		EligibleForPrizes: domain.EligibleForPayments(activePaid),
	}
	return result, true, nil
}

// GetPhaseLeaderboard returns the cached phase LeaderboardResult when available,
// or falls through to the inner Ranker and caches the result. Cache failures
// are non-fatal: a miss or a Set error falls through to the inner Ranker so a
// Redis outage never makes the leaderboard unavailable.
func (s *CachedRankingService) GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) (*LeaderboardResult, error) {
	key := cacheKeyPhaseLeaderboard(quinielaID, phase)
	if cached, ok := cacheGet[*LeaderboardResult](ctx, s.store, key, s.log); ok {
		return cached, nil
	}
	result, err := s.inner.GetPhaseLeaderboard(ctx, quinielaID, phase)
	if err != nil {
		return nil, err
	}
	if result != nil && len(result.Entries) > 0 {
		cacheSet(ctx, s.store, key, result, s.effectiveTTL(), s.log)
	}
	return result, nil
}

// GetLeaderboardWithRoundBreakdown delegates to the inner Ranker without caching.
// Round breakdown results contain per-round detail that changes frequently and
// is only used by the tournament detail UI; caching adds marginal value versus
// the complexity of keying and invalidating a separate cache entry.
func (s *CachedRankingService) GetLeaderboardWithRoundBreakdown(ctx context.Context, quinielaID int) (*LeaderboardResult, error) {
	return s.inner.GetLeaderboardWithRoundBreakdown(ctx, quinielaID)
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

// InvalidateAfterScoring implements PostScoringInvalidator: it evicts all
// cached leaderboard entries (overall and all phases) for each quiniela in
// quinielaIDs. Called by the API server if it ever needs to invalidate its
// own in-process cache after an inline scoring operation.
func (s *CachedRankingService) InvalidateAfterScoring(ctx context.Context, quinielaIDs []int) {
	for _, id := range quinielaIDs {
		s.InvalidateLeaderboard(ctx, id)
	}
}

var _ PostScoringInvalidator = (*CachedRankingService)(nil)

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
