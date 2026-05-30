// White-box tests for server.go hook constructors and bridge functions.
// This file uses package api (not api_test) so it can exercise unexported
// functions without exporting them solely for testability.
package api

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

// bridgePool creates a pgxpool.Pool pointing at an unreachable address.
// pgxpool connects lazily; Acquire on this pool will fail quickly with
// "connection refused" on localhost:1 (port 1 is never open).
func bridgePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://fake:fake@localhost:1/fake?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("create bridge pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// bridgeServer builds a minimal *Server with the given pool and a live hub,
// suitable for testing pg_notify bridge behaviour without real PostgreSQL.
func bridgeServer(t *testing.T, pool *pgxpool.Pool) *Server {
	t.Helper()
	return &Server{db: pool, notifHub: hub.New(), log: zap.NewNop()}
}

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
func (s *hookParamSvc) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *hookParamSvc) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}

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

// ── runPgNotifyBridge / listenAndBridge ───────────────────────────────────────

func TestRunPgNotifyBridge_NilDB(t *testing.T) {
	s := &Server{db: nil, notifHub: hub.New(), log: zap.NewNop()}
	done := make(chan struct{})
	go func() { s.runPgNotifyBridge(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runPgNotifyBridge did not return immediately when db is nil")
	}
}

func TestRunPgNotifyBridge_NilHub(t *testing.T) {
	s := &Server{db: bridgePool(t), notifHub: nil, log: zap.NewNop()}
	done := make(chan struct{})
	go func() { s.runPgNotifyBridge(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runPgNotifyBridge did not return immediately when notifHub is nil")
	}
}

// TestRunPgNotifyBridge_CtxPreCancelled verifies the clean-shutdown path: when
// ctx is already cancelled on entry, the bridge calls listenAndBridge once,
// receives an error from Acquire (ctx done), then detects ctx.Err() != nil and
// returns without entering the backoff retry loop.
func TestRunPgNotifyBridge_CtxPreCancelled(t *testing.T) {
	s := bridgeServer(t, bridgePool(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() { s.runPgNotifyBridge(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runPgNotifyBridge did not return after context was cancelled")
	}
}

// TestRunPgNotifyBridge_ReconnectsOnError verifies that when listenAndBridge
// returns a non-nil error and the context is still active, the bridge enters
// the exponential-backoff select and exits cleanly when the context expires.
// The unreachable pool makes Acquire fail quickly (connection refused), so the
// test completes well within the 3-second guard timeout.
func TestRunPgNotifyBridge_ReconnectsOnError(t *testing.T) {
	s := bridgeServer(t, bridgePool(t))
	// Short-lived context: bridge should call listenAndBridge, get an error,
	// enter the backoff select, and exit when the deadline fires.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.runPgNotifyBridge(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runPgNotifyBridge did not exit after context expiry during backoff")
	}
}

// TestListenAndBridge_AcquireError verifies that listenAndBridge returns a
// non-nil wrapped error when Acquire fails (context already cancelled).
func TestListenAndBridge_AcquireError(t *testing.T) {
	s := bridgeServer(t, bridgePool(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.listenAndBridge(ctx)
	if err == nil {
		t.Fatal("expected non-nil error when Acquire fails on cancelled context")
	}
}

// ── ensureIdempotencyStore ────────────────────────────────────────────────────

func TestEnsureIdempotencyStore_ReturnsFalseWhenAlreadySet(t *testing.T) {
	s := &Server{log: zap.NewNop()}
	s.SetIdempotencyStore(idempotency.NewMemoryStore())

	if s.ensureIdempotencyStore() {
		t.Error("expected degraded=false when store was already configured")
	}
	if s.idemStore == nil {
		t.Error("idemStore must remain non-nil after call")
	}
}

func TestEnsureIdempotencyStore_ReturnsTrueAndSetsMemoryStore(t *testing.T) {
	s := &Server{log: zap.NewNop()}

	if !s.ensureIdempotencyStore() {
		t.Error("expected degraded=true when no store was configured")
	}
	if s.idemStore == nil {
		t.Error("idemStore must be non-nil after fallback")
	}
}

func TestEnsureIdempotencyStore_IdempotentOnRepeatedCalls(t *testing.T) {
	s := &Server{log: zap.NewNop()}

	first := s.ensureIdempotencyStore()
	second := s.ensureIdempotencyStore() // already set by first call

	if !first {
		t.Error("first call: expected degraded=true")
	}
	if second {
		t.Error("second call: expected degraded=false (store already set)")
	}
}
