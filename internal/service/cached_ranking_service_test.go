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

func (r *stubRanker) GetLeaderboard(_ context.Context, _ int) ([]*domain.LeaderboardEntry, error) {
	r.called++
	return r.entries, r.err
}

func (r *stubRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) ([]*domain.LeaderboardEntry, error) {
	r.called++
	return r.entries, r.err
}

// ── GetLeaderboard ────────────────────────────────────────────────────────────

func TestCachedRankingService_GetLeaderboard_CacheHit_ReturnsWithoutCallingInner(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{}
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: "Alice"}, TotalPoints: 10, Rank: 1},
	}
	st.seed(cacheKeyLeaderboard(5), entries)

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 5)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry from cache, got %d", len(got))
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
	if len(got) != 1 {
		t.Errorf("expected 1 entry from inner, got %d", len(got))
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
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d entries", len(got))
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
	if len(got) != 1 {
		t.Errorf("expected 1 entry from inner after cache error, got %d", len(got))
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
	if len(got) != 1 {
		t.Errorf("expected 1 entry despite cache set error, got %d", len(got))
	}
}

// ── InvalidateLeaderboard ─────────────────────────────────────────────────────

func TestCachedRankingService_InvalidateLeaderboard_DeletesAllEightKeys(t *testing.T) {
	// Seed all 8 keys so we can verify each one is evicted.
	st := newStubCache()
	quinielaID := 11
	st.seed(cacheKeyLeaderboard(quinielaID), []*domain.LeaderboardEntry{})
	for _, phase := range domain.AllMatchPhases {
		st.seed(cacheKeyPhaseLeaderboard(quinielaID, phase), []*domain.LeaderboardEntry{})
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
	st.seed(cacheKeyPhaseLeaderboard(5, phase), entries)

	svc := NewCachedRankingService(ranker, st, 60*time.Second, zap.NewNop())
	got, err := svc.GetPhaseLeaderboard(context.Background(), 5, phase)
	if err != nil {
		t.Fatalf(cachedUnexpectedErrorFmt, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry from cache, got %d", len(got))
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
	if len(got) != 1 {
		t.Errorf("expected 1 entry from inner, got %d", len(got))
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
	if len(got) != 1 {
		t.Errorf("expected 1 entry from inner after cache error, got %d", len(got))
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
	if len(got) != 1 {
		t.Errorf("expected 1 entry despite cache set error, got %d", len(got))
	}
}

// ── UpdateTTL ─────────────────────────────────────────────────────────────────

func TestCachedRankingService_UpdateTTL_NewEntriesUseUpdatedTTL(t *testing.T) {
	// Verify that cache writes after UpdateTTL use the new duration.
	// Each GetLeaderboard for a distinct quinielaID is a guaranteed cache miss
	// (the stub store starts empty), so each call triggers a Set with the
	// current TTL - no eviction step required.
	st := &stubTTLCacheStore{stubCacheStore: newStubCache()}
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: "Alice"}, TotalPoints: 10, Rank: 1},
	}
	svc := NewCachedRankingService(&stubRanker{entries: entries}, st, 60*time.Second, zap.NewNop())

	_, _ = svc.GetLeaderboard(context.Background(), 1) // quiniela 1 - misses, writes with 60s TTL
	if len(st.ttls) == 0 || st.ttls[0] != 60*time.Second {
		t.Fatalf("expected initial TTL=60s, got %v", st.ttls)
	}

	svc.UpdateTTL(10 * time.Second)

	_, _ = svc.GetLeaderboard(context.Background(), 2) // quiniela 2 - fresh miss, writes with 10s TTL
	if len(st.ttls) < 2 || st.ttls[len(st.ttls)-1] != 10*time.Second {
		t.Errorf("expected updated TTL=10s after UpdateTTL, got %v", st.ttls)
	}
}

// stubTTLCacheStore extends stubCacheStore to record the TTL of each Set call.
type stubTTLCacheStore struct {
	*stubCacheStore
	ttls []time.Duration
}

func (s *stubTTLCacheStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	s.ttls = append(s.ttls, ttl)
	return s.stubCacheStore.Set(ctx, key, value, ttl)
}

// ── InvalidateAll ─────────────────────────────────────────────────────────────

func TestCachedRankingService_InvalidateAll_CallsFlushByPrefix(t *testing.T) {
	// A store that implements PrefixFlusher should have FlushByPrefix called
	// with the "leaderboard:" prefix.
	pf := &spyPrefixFlusher{}
	svc := NewCachedRankingService(&stubRanker{}, pf, 60*time.Second, zap.NewNop())

	svc.InvalidateAll(context.Background())

	if len(pf.prefixes) != 1 || pf.prefixes[0] != "leaderboard:" {
		t.Errorf("expected FlushByPrefix(%q), got %v", "leaderboard:", pf.prefixes)
	}
}

func TestCachedRankingService_InvalidateAll_NonPrefixFlusherStore_IsNoop(t *testing.T) {
	// stubCacheStore does not implement PrefixFlusher; InvalidateAll must not panic.
	svc := NewCachedRankingService(&stubRanker{}, newStubCache(), 60*time.Second, zap.NewNop())
	svc.InvalidateAll(context.Background()) // should not panic or error
}

func TestCachedRankingService_InvalidateAll_FlushError_NonFatal(t *testing.T) {
	pf := &spyPrefixFlusher{err: errors.New("redis error")}
	svc := NewCachedRankingService(&stubRanker{}, pf, 60*time.Second, zap.NewNop())
	svc.InvalidateAll(context.Background()) // must not panic
}

// ── InvalidateAfterScoring ────────────────────────────────────────────────────

func TestCachedRankingService_InvalidateAfterScoring_CallsDeleteForEachID(t *testing.T) {
	st := newStubCache()
	svc := NewCachedRankingService(&stubRanker{}, st, 60*time.Second, zap.NewNop())
	svc.InvalidateAfterScoring(context.Background(), []int{1, 2, 3})

	wantCount := 3 * (1 + len(domain.AllMatchPhases))
	if len(st.deleted) != wantCount {
		t.Errorf("expected %d keys deleted (3 IDs × %d keys each), got %d",
			wantCount, 1+len(domain.AllMatchPhases), len(st.deleted))
	}
}

func TestCachedRankingService_InvalidateAfterScoring_EmptyIDs_NoDeleteCalls(t *testing.T) {
	st := newStubCache()
	svc := NewCachedRankingService(&stubRanker{}, st, 60*time.Second, zap.NewNop())
	svc.InvalidateAfterScoring(context.Background(), nil)
	if len(st.deleted) != 0 {
		t.Errorf("expected no Delete calls for empty IDs, got %d", len(st.deleted))
	}
}

// spyPrefixFlusher implements both cache.Store and cache.PrefixFlusher.
type spyPrefixFlusher struct {
	prefixes []string
	err      error
}

func (s *spyPrefixFlusher) Get(_ context.Context, _ string, _ any) error {
	return cache.ErrCacheMiss
}
func (s *spyPrefixFlusher) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (s *spyPrefixFlusher) Delete(_ context.Context, _ ...string) error { return nil }
func (s *spyPrefixFlusher) FlushByPrefix(_ context.Context, prefix string) error {
	s.prefixes = append(s.prefixes, prefix)
	return s.err
}
