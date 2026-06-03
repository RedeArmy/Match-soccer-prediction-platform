package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

// ── snapshot / user / member stubs for snapshot-fallback tests ───────────────

type snapFallbackSnapshotRepo struct {
	snap *domain.LeaderboardSnapshot
	err  error
}

func (r *snapFallbackSnapshotRepo) GetLatest(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
	return r.snap, r.err
}
func (r *snapFallbackSnapshotRepo) Create(_ context.Context, _ *domain.LeaderboardSnapshot) error {
	return nil
}
func (r *snapFallbackSnapshotRepo) ListByQuiniela(_ context.Context, _, _ int) ([]*domain.LeaderboardSnapshot, error) {
	return nil, nil
}

const (
	cachedUnexpectedErrorFmt = "unexpected error: %v"
	fmtInnerCalledOnce       = "expected inner called once, called %d times"
)

// ── stubRanker ────────────────────────────────────────────────────────────────

type stubRanker struct {
	entries []*domain.LeaderboardEntry
	err     error
	called  int
}

func (r *stubRanker) GetLeaderboard(_ context.Context, _ int) (*LeaderboardResult, error) {
	r.called++
	if r.err != nil {
		return nil, r.err
	}
	return &LeaderboardResult{Entries: r.entries}, nil
}

func (r *stubRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) (*LeaderboardResult, error) {
	r.called++
	if r.err != nil {
		return nil, r.err
	}
	return &LeaderboardResult{Entries: r.entries}, nil
}

// spyPrefixFlusher implements both cache.Store and cache.PrefixFlusher for
// testing FlushByPrefix calls.
type spyPrefixFlusher struct {
	prefixes []string
	err      error
}

func (s *spyPrefixFlusher) Get(_ context.Context, _ string, _ any) error { return nil }
func (s *spyPrefixFlusher) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (s *spyPrefixFlusher) Delete(_ context.Context, _ ...string) error { return nil }
func (s *spyPrefixFlusher) FlushByPrefix(_ context.Context, prefix string) error {
	s.prefixes = append(s.prefixes, prefix)
	return s.err
}

var _ cache.Store = (*spyPrefixFlusher)(nil)
var _ cache.PrefixFlusher = (*spyPrefixFlusher)(nil)

// ── GetLeaderboard ────────────────────────────────────────────────────────────

func TestCachedRankingService_GetLeaderboard_CacheHit_ReturnsWithoutCallingInner(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{}
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: "Alice"}, TotalPoints: 10, Rank: 1},
	}
	st.seed(cacheKeyLeaderboard(5), &LeaderboardResult{Entries: entries})

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 5)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry from cache, got %d", len(got.Entries))
	}
	if ranker.called != 0 {
		t.Errorf("inner should not be called on cache hit, called %d times", ranker.called)
	}
}

func TestCachedRankingService_GetLeaderboard_CacheMiss_CallsInnerAndSetsCache(t *testing.T) {
	st := newStubCache()
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 2, Name: "Bob"}, TotalPoints: 20, Rank: 1},
	}
	ranker := &stubRanker{entries: entries}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 7)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry from inner, got %d", len(got.Entries))
	}
	if ranker.called != 1 {
		t.Errorf(fmtInnerCalledOnce, ranker.called)
	}
	if st.setCalls != 1 {
		t.Errorf("expected 1 cache Set call, got %d", st.setCalls)
	}
}

func TestCachedRankingService_GetLeaderboard_EmptyResult_NotCached(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{entries: []*domain.LeaderboardEntry{}}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 3)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got.Entries))
	}
	if st.setCalls != 0 {
		t.Errorf("empty results must not be cached, got %d Set calls", st.setCalls)
	}
}

func TestCachedRankingService_GetLeaderboard_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{err: errors.New("db error")}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	_, err := svc.GetLeaderboard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from inner Ranker, got nil")
	}
}

func TestCachedRankingService_GetLeaderboard_CacheGetError_FallsThroughToInner(t *testing.T) {
	st := newStubCache()
	st.getErr = errors.New("redis unavailable")
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 3, Name: "Carlos"}, TotalPoints: 15, Rank: 1},
	}
	ranker := &stubRanker{entries: entries}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 9)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry from inner after cache error, got %d", len(got.Entries))
	}
	if ranker.called != 1 {
		t.Errorf(fmtInnerCalledOnce, ranker.called)
	}
}

func TestCachedRankingService_GetLeaderboard_SetError_StillReturnsData(t *testing.T) {
	st := newStubCache()
	st.setErr = errors.New("redis write failed")
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 4, Name: "Diana"}, TotalPoints: 5, Rank: 1},
	}
	ranker := &stubRanker{entries: entries}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 2)
	if err != nil {
		t.Fatalf("set error must not propagate, got: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry despite cache set error, got %d", len(got.Entries))
	}
}

// ── InvalidateLeaderboard ─────────────────────────────────────────────────────

func TestCachedRankingService_InvalidateLeaderboard_DeletesAllEightKeys(t *testing.T) {
	// Seed all 8 keys so we can verify each one is evicted.
	st := newStubCache()
	quinielaID := 11
	st.seed(cacheKeyLeaderboard(quinielaID), &LeaderboardResult{})
	for _, phase := range domain.AllMatchPhases {
		st.seed(cacheKeyPhaseLeaderboard(quinielaID, phase), &LeaderboardResult{})
	}

	svc := NewCachedRankingService(&stubRanker{}, st, 60*time.Second, zap.NewNop())
	svc.InvalidateLeaderboard(context.Background(), quinielaID)

	wantCount := 1 + len(domain.AllMatchPhases) // 1 overall + 7 phase keys
	if len(st.deleted) != wantCount {
		t.Fatalf("expected %d keys deleted, got %d: %v", wantCount, len(st.deleted), st.deleted)
	}

	deleted := make(map[string]bool, len(st.deleted))
	for _, k := range st.deleted {
		deleted[k] = true
	}

	if !deleted[cacheKeyLeaderboard(quinielaID)] {
		t.Errorf("overall key %q was not deleted", cacheKeyLeaderboard(quinielaID))
	}
	for _, phase := range domain.AllMatchPhases {
		k := cacheKeyPhaseLeaderboard(quinielaID, phase)
		if !deleted[k] {
			t.Errorf("phase key %q was not deleted", k)
		}
	}
}

func TestCachedRankingService_InvalidateLeaderboard_DeleteError_NonFatal(t *testing.T) {
	st := newStubCache()
	st.delErr = errors.New("redis error")
	ranker := &stubRanker{}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	// Must not panic or return an error - the method has no return value.
	svc.InvalidateLeaderboard(context.Background(), 4)
}

// ── GetPhaseLeaderboard ───────────────────────────────────────────────────────

func TestCachedRankingService_GetPhaseLeaderboard_CacheHit_ReturnsWithoutCallingInner(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{}
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: "Alice"}, TotalPoints: 5, Rank: 1},
	}
	phase := domain.PhaseGroupStage
	st.seed(cacheKeyPhaseLeaderboard(5, phase), &LeaderboardResult{Entries: entries})

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetPhaseLeaderboard(context.Background(), 5, phase)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry from cache, got %d", len(got.Entries))
	}
	if ranker.called != 0 {
		t.Errorf("inner should not be called on cache hit, called %d times", ranker.called)
	}
}

func TestCachedRankingService_GetPhaseLeaderboard_CacheMiss_CallsInnerAndSetsCache(t *testing.T) {
	st := newStubCache()
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 2, Name: "Bob"}, TotalPoints: 20, Rank: 1},
	}
	ranker := &stubRanker{entries: entries}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetPhaseLeaderboard(context.Background(), 7, domain.PhaseFinal)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry from inner, got %d", len(got.Entries))
	}
	if ranker.called != 1 {
		t.Errorf(fmtInnerCalledOnce, ranker.called)
	}
	if st.setCalls != 1 {
		t.Errorf("expected 1 cache Set call, got %d", st.setCalls)
	}
}

func TestCachedRankingService_GetPhaseLeaderboard_EmptyResult_NotCached(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{entries: []*domain.LeaderboardEntry{}}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	_, err := svc.GetPhaseLeaderboard(context.Background(), 3, domain.PhaseRoundOf16)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if st.setCalls != 0 {
		t.Errorf("empty results must not be cached, got %d Set calls", st.setCalls)
	}
}

func TestCachedRankingService_GetPhaseLeaderboard_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{err: errors.New("db error")}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	_, err := svc.GetPhaseLeaderboard(context.Background(), 1, domain.PhaseGroupStage)
	if err == nil {
		t.Fatal("expected error from inner Ranker, got nil")
	}
}

func TestCachedRankingService_GetPhaseLeaderboard_CacheGetError_FallsThroughToInner(t *testing.T) {
	st := newStubCache()
	st.getErr = errors.New("redis unavailable")
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 3, Name: "Carlos"}, TotalPoints: 15, Rank: 1},
	}
	ranker := &stubRanker{entries: entries}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetPhaseLeaderboard(context.Background(), 9, domain.PhaseSemiFinal)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry from inner after cache error, got %d", len(got.Entries))
	}
}

func TestCachedRankingService_GetPhaseLeaderboard_CacheKeyContainsPhase(t *testing.T) {
	// Verify that different phases produce distinct cache keys so phase
	// leaderboards do not collide with each other or with the overall leaderboard.
	keyGroup := cacheKeyPhaseLeaderboard(1, domain.PhaseGroupStage)
	keyFinal := cacheKeyPhaseLeaderboard(1, domain.PhaseFinal)
	keyOverall := cacheKeyLeaderboard(1)

	if keyGroup == keyFinal {
		t.Errorf("group_stage and final cache keys must differ, both: %q", keyGroup)
	}
	if keyGroup == keyOverall {
		t.Errorf("phase key must differ from overall key, both: %q", keyGroup)
	}
}

func TestCachedRankingService_GetPhaseLeaderboard_SetError_StillReturnsData(t *testing.T) {
	st := newStubCache()
	st.setErr = errors.New("redis write failed")
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 4, Name: "Diana"}, TotalPoints: 5, Rank: 1},
	}
	ranker := &stubRanker{entries: entries}

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetPhaseLeaderboard(context.Background(), 2, domain.PhaseQuarterFinal)
	if err != nil {
		t.Fatalf("set error must not propagate, got: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Errorf("expected 1 entry despite cache set error, got %d", len(got.Entries))
	}
}

// ── UpdateTTL / effectiveTTL ──────────────────────────────────────────────────

// ttlTrackingCache wraps stubCacheStore and records the TTL passed to the most
// recent Set call. Used to verify that UpdateTTL propagates to cache writes.
type ttlTrackingCache struct {
	*stubCacheStore
	lastSetTTL time.Duration
}

func (c *ttlTrackingCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	c.lastSetTTL = ttl
	return c.stubCacheStore.Set(ctx, key, value, ttl)
}

func TestCachedRankingService_UpdateTTL_AffectsCacheWriteTTL(t *testing.T) {
	t.Parallel()
	// Arrange: construct with an initial TTL, then change it.
	inner := &stubRanker{entries: []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, TotalPoints: 10, Rank: 1},
	}}
	spy := &ttlTrackingCache{stubCacheStore: newStubCache()}
	initialTTL := 60 * time.Second
	updatedTTL := 120 * time.Second

	svc := NewCachedRankingService(inner, spy, initialTTL, zap.NewNop())

	// First call: cache miss → stores with initialTTL.
	if _, err := svc.GetLeaderboard(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if spy.lastSetTTL != initialTTL {
		t.Errorf("first Set TTL: got %v; want %v", spy.lastSetTTL, initialTTL)
	}

	// UpdateTTL, then query a fresh quiniela ID (guaranteed cache miss since it
	// was never populated). stubCacheStore.Delete is a spy that does not evict
	// from the data map, so a different quiniela ID is the reliable approach.
	svc.UpdateTTL(updatedTTL)

	if _, err := svc.GetLeaderboard(context.Background(), 2); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if spy.lastSetTTL != updatedTTL {
		t.Errorf("second Set TTL after UpdateTTL: got %v; want %v", spy.lastSetTTL, updatedTTL)
	}
}

// ── WithSnapshotFallback ──────────────────────────────────────────────────────

func TestCachedRankingService_SnapshotFallback_ServedOnColdCacheMiss(t *testing.T) {
	t.Parallel()
	snap := &domain.LeaderboardSnapshot{
		QuinielaID: 1,
		Entries: []domain.LeaderboardSnapshotEntry{
			{UserID: 10, Rank: 1, TotalPoints: 15, PrizeWinner: true},
		},
	}
	users := []*domain.User{{ID: 10, Name: "Alice"}}

	inner := &stubRanker{} // never called when snapshot hit
	st := newStubCache()
	svc := NewCachedRankingService(inner, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{snap: snap},
			&stubUserRepo{users: users},
			&stubMemberRepo{activeCount: 5},
		),
	)

	result, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry from snapshot, got %d", len(result.Entries))
	}
	if result.Entries[0].User.Name != "Alice" {
		t.Errorf("expected user Alice, got %q", result.Entries[0].User.Name)
	}
	if result.Entries[0].Rank != 1 {
		t.Errorf("expected rank 1, got %d", result.Entries[0].Rank)
	}
	if result.ActivePaidMembers != 5 {
		t.Errorf("expected 5 active paid members, got %d", result.ActivePaidMembers)
	}
	if inner.called != 0 {
		t.Errorf("inner ranker should not be called on snapshot hit, called %d times", inner.called)
	}
}

func TestCachedRankingService_SnapshotFallback_FallsBackToInnerWhenNoSnapshot(t *testing.T) {
	t.Parallel()
	inner := &stubRanker{entries: []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: "Bob"}, TotalPoints: 10, Rank: 1},
	}}
	st := newStubCache()
	svc := NewCachedRankingService(inner, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{snap: nil}, // no snapshot exists yet
			&stubUserRepo{},
			&stubMemberRepo{activeCount: 3},
		),
	)

	result, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called != 1 {
		t.Errorf("inner ranker should be called once when snapshot is nil, called %d times", inner.called)
	}
	if len(result.Entries) != 1 || result.Entries[0].User.Name != "Bob" {
		t.Errorf("expected result from inner ranker")
	}
}

func TestCachedRankingService_SnapshotFallback_FallsBackToInnerOnSnapshotRepoError(t *testing.T) {
	t.Parallel()
	inner := &stubRanker{entries: []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: "Carol"}, TotalPoints: 5, Rank: 1},
	}}
	st := newStubCache()
	svc := NewCachedRankingService(inner, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{err: errors.New("db error")},
			&stubUserRepo{},
			&stubMemberRepo{},
		),
	)

	result, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called != 1 {
		t.Errorf("inner ranker should be called once on snapshot repo error, called %d", inner.called)
	}
	if len(result.Entries) == 0 || result.Entries[0].User.Name != "Carol" {
		t.Error("expected result from inner ranker on snapshot error")
	}
}

func TestCachedRankingService_SnapshotFallback_FallsBackOnUserEnrichmentError(t *testing.T) {
	t.Parallel()
	snap := &domain.LeaderboardSnapshot{
		QuinielaID: 1,
		Entries:    []domain.LeaderboardSnapshotEntry{{UserID: 10, Rank: 1, TotalPoints: 5}},
	}
	inner := &stubRanker{entries: []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 10, Name: "Dave"}, TotalPoints: 5, Rank: 1},
	}}
	st := newStubCache()
	svc := NewCachedRankingService(inner, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{snap: snap},
			&stubUserRepo{err: errors.New("db error")},
			&stubMemberRepo{},
		),
	)

	result, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called != 1 {
		t.Errorf("inner ranker should be called once on user enrichment error, called %d", inner.called)
	}
	_ = result
}

func TestCachedRankingService_SnapshotFallback_FallsBackOnCountActivePaidError(t *testing.T) {
	t.Parallel()
	snap := &domain.LeaderboardSnapshot{
		QuinielaID: 1,
		Entries:    []domain.LeaderboardSnapshotEntry{{UserID: 10, Rank: 1, TotalPoints: 5}},
	}
	inner := &stubRanker{entries: []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 10, Name: "Eve"}, TotalPoints: 5, Rank: 1},
	}}
	st := newStubCache()
	svc := NewCachedRankingService(inner, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{snap: snap},
			&stubUserRepo{users: []*domain.User{{ID: 10, Name: "Eve"}}},
			&stubMemberRepo{countActiveErr: errors.New("db error")}, // uses countActiveErr not err
		),
	)

	result, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called != 1 {
		t.Errorf("inner ranker should be called once on CountActivePaid error, called %d", inner.called)
	}
	_ = result
}

func TestCachedRankingService_SnapshotFallback_CachesSnapshotResult(t *testing.T) {
	t.Parallel()
	snap := &domain.LeaderboardSnapshot{
		QuinielaID: 1,
		Entries:    []domain.LeaderboardSnapshotEntry{{UserID: 10, Rank: 1, TotalPoints: 8}},
	}
	inner := &stubRanker{}
	st := newStubCache()
	svc := NewCachedRankingService(inner, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{snap: snap},
			&stubUserRepo{users: []*domain.User{{ID: 10, Name: "Frank"}}},
			&stubMemberRepo{activeCount: 2},
		),
	)

	// First call: cold cache, should use snapshot and populate cache.
	if _, err := svc.GetLeaderboard(context.Background(), 1); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call: should be served from cache without calling inner.
	if _, err := svc.GetLeaderboard(context.Background(), 1); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if inner.called != 0 {
		t.Errorf("inner ranker should never be called when snapshot + cache are working, called %d", inner.called)
	}
}

func TestCachedRankingService_SnapshotFallback_SkipsDeletedUsers(t *testing.T) {
	t.Parallel()
	// Snapshot references user 99 which is soft-deleted (not in users table).
	snap := &domain.LeaderboardSnapshot{
		QuinielaID: 1,
		Entries: []domain.LeaderboardSnapshotEntry{
			{UserID: 99, Rank: 1, TotalPoints: 10},
			{UserID: 10, Rank: 2, TotalPoints: 5},
		},
	}
	// Only user 10 is returned; user 99 is absent (soft-deleted).
	users := []*domain.User{{ID: 10, Name: "Grace"}}
	st := newStubCache()
	svc := NewCachedRankingService(&stubRanker{}, st, 30*time.Second, zap.NewNop(),
		WithSnapshotFallback(
			&snapFallbackSnapshotRepo{snap: snap},
			&stubUserRepo{users: users},
			&stubMemberRepo{activeCount: 2},
		),
	)

	result, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry (soft-deleted user skipped), got %d", len(result.Entries))
	}
	if result.Entries[0].User.Name != "Grace" {
		t.Errorf("expected Grace, got %q", result.Entries[0].User.Name)
	}
}

func TestCachedRankingService_UpdateTTL_ZeroDuration_DisablesEffectiveTTL(t *testing.T) {
	t.Parallel()
	inner := &stubRanker{entries: []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 2}, TotalPoints: 5, Rank: 1},
	}}
	spy := &ttlTrackingCache{stubCacheStore: newStubCache()}
	svc := NewCachedRankingService(inner, spy, 30*time.Second, zap.NewNop())

	svc.UpdateTTL(0)
	svc.InvalidateLeaderboard(context.Background(), 2)

	if _, err := svc.GetLeaderboard(context.Background(), 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.lastSetTTL != 0 {
		t.Errorf("expected zero TTL after UpdateTTL(0), got %v", spy.lastSetTTL)
	}
}
