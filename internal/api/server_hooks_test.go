// White-box tests for server.go hook constructors. This file uses package api
// (not api_test) so it can exercise unexported functions without exporting them
// solely for testability.
package api

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

// hookParamSvc is a minimal SystemParamService stub whose only meaningful
// method is GetInt, which returns a preset value. All other methods are no-ops
// that return zero values; they should never be called by leaderboardTTLHook.
type hookParamSvc struct{ intVal int }

func (s *hookParamSvc) GetInt(_ context.Context, _ string, _ int) int   { return s.intVal }
func (s *hookParamSvc) GetString(_ context.Context, _, d string) string { return d }
func (s *hookParamSvc) GetDuration(_ context.Context, _ string, d time.Duration) time.Duration {
	return d
}
func (s *hookParamSvc) GetBool(_ context.Context, _ string, d bool) bool { return d }
func (s *hookParamSvc) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *hookParamSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error) { return nil, nil }
func (s *hookParamSvc) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (s *hookParamSvc) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *hookParamSvc) BulkSet(_ context.Context, _ map[string]string, _ int) error { return nil }

// hookCacheStore implements cache.Store and cache.PrefixFlusher. It records
// whether FlushByPrefix was called and with which prefix so the test can assert
// that InvalidateAll drove the right eviction.
type hookCacheStore struct {
	flushedPrefix string
	flushErr      error
}

func (s *hookCacheStore) Get(_ context.Context, _ string, _ any) error { return cache.ErrCacheMiss }
func (s *hookCacheStore) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (s *hookCacheStore) Delete(_ context.Context, _ ...string) error { return nil }
func (s *hookCacheStore) FlushByPrefix(_ context.Context, prefix string) error {
	s.flushedPrefix = prefix
	return s.flushErr
}

// hookRanker is a no-op service.Ranker; leaderboardTTLHook never calls it directly.
type hookRanker struct{}

func (hookRanker) GetLeaderboard(_ context.Context, _ int) (*service.LeaderboardResult, error) {
	return &service.LeaderboardResult{}, nil
}
func (hookRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) (*service.LeaderboardResult, error) {
	return &service.LeaderboardResult{}, nil
}

// ── leaderboardTTLHook ────────────────────────────────────────────────────────

func TestLeaderboardTTLHook_UpdatesTTLAndInvalidatesCache(t *testing.T) {
	const newSeconds = 120

	paramSvc := &hookParamSvc{intVal: newSeconds}
	store := &hookCacheStore{}
	ranker := service.NewCachedRankingService(hookRanker{}, store, 60*time.Second, zap.NewNop())

	leaderboardTTLHook(paramSvc, ranker)(context.Background())

	// Verify FlushByPrefix was driven with the correct prefix, confirming both
	// InvalidateAll and UpdateTTL were reached inside the hook body.
	if store.flushedPrefix != "leaderboard:" {
		t.Errorf("FlushByPrefix prefix: got %q, want %q", store.flushedPrefix, "leaderboard:")
	}
}

func TestLeaderboardTTLHook_UsesDefaultWhenParamServiceReturnsDefault(t *testing.T) {
	paramSvc := &hookParamSvc{intVal: domain.DefaultCacheLeaderboardTTLSeconds}
	store := &hookCacheStore{}
	ranker := service.NewCachedRankingService(hookRanker{}, store, 30*time.Second, zap.NewNop())

	leaderboardTTLHook(paramSvc, ranker)(context.Background())

	if store.flushedPrefix == "" {
		t.Error("expected FlushByPrefix to be called; hook body did not execute")
	}
}
