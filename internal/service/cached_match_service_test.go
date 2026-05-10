package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

const (
	fmtUnexpectedErr  = "unexpected error: %v"
	fmtMatchFromCache = "expected 1 match from cache, got %d"
	fmtInnerNotCalled = "inner should not be called on cache hit, called %d times"
	fmtMatchFromInner = "expected 1 match from inner, got %d"
	errDBMsg          = "db error"
)

// ── stubCacheStore ────────────────────────────────────────────────────────────

// stubCacheStore is an in-memory cache.Store for service-layer unit tests.
// Seed preloads JSON-serialised values so Get finds them on the first call.
type stubCacheStore struct {
	data     map[string][]byte
	getErr   error
	setErr   error
	delErr   error
	deleted  []string
	setCalls int
}

func newStubCache() *stubCacheStore {
	return &stubCacheStore{data: make(map[string][]byte)}
}

// seed JSON-encodes v and stores it under key so that Get returns a cache hit.
func (s *stubCacheStore) seed(key string, v any) {
	b, _ := json.Marshal(v)
	s.data[key] = b
}

func (s *stubCacheStore) Get(_ context.Context, key string, dest any) error {
	if s.getErr != nil {
		return s.getErr
	}
	b, ok := s.data[key]
	if !ok {
		return cache.ErrCacheMiss
	}
	return json.Unmarshal(b, dest)
}

func (s *stubCacheStore) Set(_ context.Context, key string, value any, _ time.Duration) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.setCalls++
	b, _ := json.Marshal(value)
	s.data[key] = b
	return nil
}

func (s *stubCacheStore) Delete(_ context.Context, keys ...string) error {
	if s.delErr != nil {
		return s.delErr
	}
	s.deleted = append(s.deleted, keys...)
	return nil
}

// ── stubInnerMatchSvc ─────────────────────────────────────────────────────────

// stubInnerMatchSvc is a minimal MatchService stub for cachedMatchService tests.
type stubInnerMatchSvc struct {
	match   *domain.Match
	matches []*domain.Match
	err     error
	called  int
}

func (s *stubInnerMatchSvc) ListMatches(_ context.Context) ([]*domain.Match, error) {
	s.called++
	return s.matches, s.err
}
func (s *stubInnerMatchSvc) ListMatchesByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	s.called++
	return s.matches, s.err
}
func (s *stubInnerMatchSvc) ListMatchesByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	s.called++
	return s.matches, s.err
}
func (s *stubInnerMatchSvc) CreateMatch(_ context.Context, _ *domain.Match) error { return s.err }
func (s *stubInnerMatchSvc) UpdateResult(_ context.Context, _ int, _, _ int, _ *domain.WinMethod) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubInnerMatchSvc) StartMatch(_ context.Context, _ int) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubInnerMatchSvc) GetMatch(_ context.Context, _ int) (*domain.Match, error) {
	s.called++
	return s.match, s.err
}

// ── ListMatches ───────────────────────────────────────────────────────────────

func TestCachedMatchService_ListMatches_CacheHit_ReturnsWithoutCallingInner(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{}
	matches := []*domain.Match{{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina"}}
	st.seed(cacheKeyMatchesAll, matches)

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatches(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtMatchFromCache, len(got))
	}
	if inner.called != 0 {
		t.Errorf(fmtInnerNotCalled, inner.called)
	}
}

func TestCachedMatchService_ListMatches_CacheMiss_CallsInnerAndPopulatesCache(t *testing.T) {
	st := newStubCache()
	matches := []*domain.Match{{ID: 2, HomeTeam: "France", AwayTeam: "Germany"}}
	inner := &stubInnerMatchSvc{matches: matches}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatches(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtMatchFromInner, len(got))
	}
	if inner.called != 1 {
		t.Errorf("expected inner to be called once, called %d times", inner.called)
	}
	if st.setCalls != 1 {
		t.Errorf("expected 1 cache Set call, got %d", st.setCalls)
	}
}

func TestCachedMatchService_ListMatches_CacheGetError_FallsThroughToInner(t *testing.T) {
	st := newStubCache()
	st.getErr = errors.New("redis unavailable")
	matches := []*domain.Match{{ID: 3}}
	inner := &stubInnerMatchSvc{matches: matches}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatches(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 match from inner after cache error, got %d", len(got))
	}
	if inner.called != 1 {
		t.Errorf("expected inner called once after cache error, called %d times", inner.called)
	}
}

func TestCachedMatchService_ListMatches_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{err: errors.New(errDBMsg)}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	_, err := svc.ListMatches(context.Background())
	if err == nil {
		t.Fatal("expected error from inner, got nil")
	}
}

func TestCachedMatchService_ListMatches_SetError_StillReturnsData(t *testing.T) {
	st := newStubCache()
	st.setErr = errors.New("redis write failed")
	matches := []*domain.Match{{ID: 4}}
	inner := &stubInnerMatchSvc{matches: matches}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatches(context.Background())
	if err != nil {
		t.Fatalf("set error must not propagate, got: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 match despite cache set error, got %d", len(got))
	}
}

// ── ListMatchesByPhase ────────────────────────────────────────────────────────

func TestCachedMatchService_ListMatchesByPhase_CacheHit_ReturnsWithoutCallingInner(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{}
	phase := domain.PhaseGroupStage
	matches := []*domain.Match{{ID: 5, Phase: phase}}
	st.seed(cacheKeyMatchesByPhase(phase), matches)

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatchesByPhase(context.Background(), phase)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtMatchFromCache, len(got))
	}
	if inner.called != 0 {
		t.Errorf(fmtInnerNotCalled, inner.called)
	}
}

func TestCachedMatchService_ListMatchesByPhase_CacheMiss_CallsInner(t *testing.T) {
	st := newStubCache()
	phase := domain.PhaseGroupStage
	inner := &stubInnerMatchSvc{matches: []*domain.Match{{ID: 6, Phase: phase}}}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatchesByPhase(context.Background(), phase)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtMatchFromInner, len(got))
	}
	if inner.called != 1 {
		t.Errorf("expected inner called once, called %d times", inner.called)
	}
}

func TestCachedMatchService_ListMatchesByPhase_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{err: errors.New(errDBMsg)}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	_, err := svc.ListMatchesByPhase(context.Background(), domain.PhaseGroupStage)
	if err == nil {
		t.Fatal("expected error from inner, got nil")
	}
}

// ── ListMatchesByStatus ───────────────────────────────────────────────────────

func TestCachedMatchService_ListMatchesByStatus_CacheHit_ReturnsWithoutCallingInner(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{}
	status := domain.MatchStatusScheduled
	matches := []*domain.Match{{ID: 7, Status: status}}
	st.seed(cacheKeyMatchesByStatus(status), matches)

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatchesByStatus(context.Background(), status)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtMatchFromCache, len(got))
	}
	if inner.called != 0 {
		t.Errorf(fmtInnerNotCalled, inner.called)
	}
}

func TestCachedMatchService_ListMatchesByStatus_CacheMiss_CallsInner(t *testing.T) {
	st := newStubCache()
	status := domain.MatchStatusLive
	inner := &stubInnerMatchSvc{matches: []*domain.Match{{ID: 8, Status: status}}}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.ListMatchesByStatus(context.Background(), status)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtMatchFromInner, len(got))
	}
	if inner.called != 1 {
		t.Errorf("expected inner called once, called %d times", inner.called)
	}
}

// ── CreateMatch ───────────────────────────────────────────────────────────────

func TestCachedMatchService_CreateMatch_Success_InvalidatesCache(t *testing.T) {
	st := newStubCache()
	m := &domain.Match{ID: 9, Phase: domain.PhaseGroupStage, Status: domain.MatchStatusScheduled}
	inner := &stubInnerMatchSvc{match: m}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	if err := svc.CreateMatch(context.Background(), m); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(st.deleted) == 0 {
		t.Error("expected cache keys to be invalidated after CreateMatch")
	}
}

func TestCachedMatchService_CreateMatch_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{err: errors.New(errDBMsg)}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	err := svc.CreateMatch(context.Background(), &domain.Match{})
	if err == nil {
		t.Fatal("expected error from inner CreateMatch, got nil")
	}
	if len(st.deleted) != 0 {
		t.Error("cache must not be invalidated when inner CreateMatch fails")
	}
}

// ── UpdateResult ──────────────────────────────────────────────────────────────

func TestCachedMatchService_UpdateResult_Success_InvalidatesCache(t *testing.T) {
	st := newStubCache()
	m := &domain.Match{ID: 1, Phase: domain.PhaseGroupStage, Status: domain.MatchStatusFinished}
	inner := &stubInnerMatchSvc{match: m}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.UpdateResult(context.Background(), 1, 2, 1, nil)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected non-nil match from UpdateResult")
	}
	if len(st.deleted) == 0 {
		t.Error("expected cache keys to be invalidated after UpdateResult")
	}
}

func TestCachedMatchService_UpdateResult_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{err: errors.New("match not live")}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	_, err := svc.UpdateResult(context.Background(), 1, 2, 1, nil)
	if err == nil {
		t.Fatal("expected error from inner UpdateResult, got nil")
	}
}

// ── StartMatch ────────────────────────────────────────────────────────────────

func TestCachedMatchService_StartMatch_Success_InvalidatesCache(t *testing.T) {
	st := newStubCache()
	m := &domain.Match{ID: 1, Phase: domain.PhaseGroupStage, Status: domain.MatchStatusLive}
	inner := &stubInnerMatchSvc{match: m}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.StartMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected non-nil match from StartMatch")
	}
	if len(st.deleted) == 0 {
		t.Error("expected cache keys to be invalidated after StartMatch")
	}
}

func TestCachedMatchService_StartMatch_InnerError_Propagated(t *testing.T) {
	st := newStubCache()
	inner := &stubInnerMatchSvc{err: errors.New("already live")}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	_, err := svc.StartMatch(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from inner StartMatch, got nil")
	}
}

// ── GetMatch ──────────────────────────────────────────────────────────────────

func TestCachedMatchService_GetMatch_DelegatesDirectlyToInner(t *testing.T) {
	st := newStubCache()
	m := &domain.Match{ID: 42, HomeTeam: "Spain", AwayTeam: "England"}
	inner := &stubInnerMatchSvc{match: m}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	got, err := svc.GetMatch(context.Background(), 42)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != 42 {
		t.Errorf("expected match ID 42, got %v", got)
	}
	if inner.called != 1 {
		t.Errorf("expected inner GetMatch called once, called %d times", inner.called)
	}
}

// ── invalidateMatchLists error path ──────────────────────────────────────────

func TestCachedMatchService_InvalidateMatchLists_DeleteError_NonFatal(t *testing.T) {
	st := newStubCache()
	st.delErr = errors.New("redis write failed")
	m := &domain.Match{ID: 1, Phase: domain.PhaseGroupStage, Status: domain.MatchStatusFinished}
	inner := &stubInnerMatchSvc{match: m}

	svc := NewCachedMatchService(inner, st, 5*time.Minute, zap.NewNop())
	// UpdateResult triggers invalidateMatchLists; the delete error must not propagate.
	_, err := svc.UpdateResult(context.Background(), 1, 2, 1, nil)
	if err != nil {
		t.Fatalf("delete error must not propagate from invalidateMatchLists, got: %v", err)
	}
}
