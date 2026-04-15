package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
	got, err := svc.GetLeaderboard(context.Background(), 2)
	if err != nil {
		t.Fatalf("set error must not propagate, got: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry despite cache set error, got %d", len(got))
	}
}

// ── InvalidateLeaderboard ─────────────────────────────────────────────────────

func TestCachedRankingService_InvalidateLeaderboard_DeletesKey(t *testing.T) {
	st := newStubCache()
	ranker := &stubRanker{}

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
	svc.InvalidateLeaderboard(context.Background(), 11)

	wantKey := fmt.Sprintf("leaderboard:%d", 11)
	found := false
	for _, k := range st.deleted {
		if k == wantKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected key %q to be deleted, deleted keys: %v", wantKey, st.deleted)
	}
}

func TestCachedRankingService_InvalidateLeaderboard_DeleteError_NonFatal(t *testing.T) {
	st := newStubCache()
	st.delErr = errors.New("redis error")
	ranker := &stubRanker{}

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
	// Must not panic or return an error — the method has no return value.
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
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

	svc := NewCachedRankingService(ranker, st, zap.NewNop())
	got, err := svc.GetPhaseLeaderboard(context.Background(), 2, domain.PhaseQuarterFinal)
	if err != nil {
		t.Fatalf("set error must not propagate, got: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 entry despite cache set error, got %d", len(got))
	}
}
