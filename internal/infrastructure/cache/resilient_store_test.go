package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// ── stub store ────────────────────────────────────────────────────────────────

type stubStore struct {
	getErr error
	setErr error
	delErr error

	getCallCount int
	setCallCount int
	delCallCount int
}

func (s *stubStore) Get(_ context.Context, _ string, dest interface{}) error {
	s.getCallCount++
	return s.getErr
}
func (s *stubStore) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) error {
	s.setCallCount++
	return s.setErr
}
func (s *stubStore) Delete(_ context.Context, _ ...string) error {
	s.delCallCount++
	return s.delErr
}
func (s *stubStore) FlushByPrefix(_ context.Context, _ string) error {
	return nil
}

var errNet = errors.New("connection refused")

// openBreaker returns a Breaker that is already in the Open state.
func openBreaker(t *testing.T) *breaker.Breaker {
	t.Helper()
	b := breaker.New(t.Name(), 1, time.Hour)
	_ = b.Call(func() error { return errNet })
	return b
}

// closedBreaker returns a fresh Breaker in the Closed state.
func closedBreaker(t *testing.T) *breaker.Breaker {
	t.Helper()
	return breaker.New(t.Name(), 5, time.Second)
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestResilientStore_Get_CircuitOpen_ReturnsCacheMiss(t *testing.T) {
	inner := &stubStore{}
	rs := cache.NewResilientStore(inner, openBreaker(t), zap.NewNop())

	err := rs.Get(context.Background(), "k", new(string))
	if !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss when circuit open, got %v", err)
	}
	if inner.getCallCount != 0 {
		t.Errorf("inner.Get must not be called when circuit is open, called %d times", inner.getCallCount)
	}
}

func TestResilientStore_Get_CacheMiss_NotCountedAsFailure(t *testing.T) {
	inner := &stubStore{getErr: cache.ErrCacheMiss}
	b := closedBreaker(t)
	rs := cache.NewResilientStore(inner, b, zap.NewNop())

	for i := 0; i < 10; i++ {
		_ = rs.Get(context.Background(), "k", new(string))
	}
	if b.CurrentState() != breaker.StateClosed {
		t.Errorf("ErrCacheMiss must not open the circuit; state = %s", b.CurrentState())
	}
}

func TestResilientStore_Get_NetworkError_CountsAsFailure(t *testing.T) {
	inner := &stubStore{getErr: errNet}
	b := breaker.New(t.Name(), 2, time.Second)
	rs := cache.NewResilientStore(inner, b, zap.NewNop())

	_ = rs.Get(context.Background(), "k", new(string))
	_ = rs.Get(context.Background(), "k", new(string))
	if b.CurrentState() != breaker.StateOpen {
		t.Errorf("expected circuit Open after 2 network errors, got %s", b.CurrentState())
	}
}

func TestResilientStore_Get_Success_ReturnsNil(t *testing.T) {
	inner := &stubStore{getErr: nil}
	rs := cache.NewResilientStore(inner, closedBreaker(t), zap.NewNop())

	if err := rs.Get(context.Background(), "k", new(string)); err != nil {
		t.Errorf("expected nil on success, got %v", err)
	}
}

// ── Set ───────────────────────────────────────────────────────────────────────

func TestResilientStore_Set_CircuitOpen_SilentlyDrops(t *testing.T) {
	inner := &stubStore{}
	rs := cache.NewResilientStore(inner, openBreaker(t), zap.NewNop())

	err := rs.Set(context.Background(), "k", "v", time.Minute)
	if err != nil {
		t.Errorf("expected nil (silent drop) when circuit open, got %v", err)
	}
	if inner.setCallCount != 0 {
		t.Errorf("inner.Set must not be called when circuit is open")
	}
}

func TestResilientStore_Set_NetworkError_PropagatesAndCountsFailure(t *testing.T) {
	inner := &stubStore{setErr: errNet}
	b := breaker.New(t.Name(), 1, time.Second)
	rs := cache.NewResilientStore(inner, b, zap.NewNop())

	err := rs.Set(context.Background(), "k", "v", time.Minute)
	if !errors.Is(err, errNet) {
		t.Errorf("expected errNet on Set failure, got %v", err)
	}
	if b.CurrentState() != breaker.StateOpen {
		t.Errorf("expected circuit Open after failed Set, got %s", b.CurrentState())
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestResilientStore_Delete_CircuitOpen_SilentlySkips(t *testing.T) {
	inner := &stubStore{}
	rs := cache.NewResilientStore(inner, openBreaker(t), zap.NewNop())

	err := rs.Delete(context.Background(), "k1", "k2")
	if err != nil {
		t.Errorf("expected nil (silent skip) when circuit open, got %v", err)
	}
	if inner.delCallCount != 0 {
		t.Errorf("inner.Delete must not be called when circuit is open")
	}
}

func TestResilientStore_Delete_NoKeys_ReturnsNilWithoutCall(t *testing.T) {
	inner := &stubStore{}
	rs := cache.NewResilientStore(inner, closedBreaker(t), zap.NewNop())

	if err := rs.Delete(context.Background()); err != nil {
		t.Errorf("expected nil for empty Delete, got %v", err)
	}
	if inner.delCallCount != 0 {
		t.Errorf("inner.Delete should not be called with no keys")
	}
}

// ── FlushByPrefix ─────────────────────────────────────────────────────────────

func TestResilientStore_FlushByPrefix_CircuitOpen_SilentlySkips(t *testing.T) {
	inner := &stubStore{}
	rs := cache.NewResilientStore(inner, openBreaker(t), zap.NewNop())

	if err := rs.FlushByPrefix(context.Background(), "lb:"); err != nil {
		t.Errorf("expected nil when circuit open, got %v", err)
	}
}

func TestResilientStore_FlushByPrefix_InnerNotPrefixFlusher_ReturnsNil(t *testing.T) {
	// stubStore does implement FlushByPrefix above, but let's use a plain Store.
	type plainStore struct{ cache.Store }
	inner := &plainStore{}
	b := closedBreaker(t)
	rs := cache.NewResilientStore(inner, b, zap.NewNop())

	if err := rs.FlushByPrefix(context.Background(), "lb:"); err != nil {
		t.Errorf("expected nil when inner is not a PrefixFlusher, got %v", err)
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestResilientStore_ImplementsStore(t *testing.T) {
	var _ cache.Store = (*cache.ResilientStore)(nil)
}

func TestResilientStore_ImplementsPrefixFlusher(t *testing.T) {
	var _ cache.PrefixFlusher = (*cache.ResilientStore)(nil)
}
