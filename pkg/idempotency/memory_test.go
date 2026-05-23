package idempotency_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

func newMemStore() *idempotency.MemoryStore { return idempotency.NewMemoryStore() }

// ── Load ──────────────────────────────────────────────────────────────────────

func TestMemoryStore_Load_UnknownKey_ReturnsFalse(t *testing.T) {
	t.Parallel()
	s := newMemStore()

	_, found, err := s.Load(context.Background(), "no-such-key")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if found {
		t.Error("Load on unknown key: want found=false, got true")
	}
}

func TestMemoryStore_Load_ExpiredKey_ReturnsFalse(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "exp", time.Nanosecond)
	time.Sleep(time.Millisecond) // let the entry expire

	_, found, err := s.Load(ctx, "exp")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Error("Load on expired key: want found=false, got true")
	}
}

// ── Reserve ───────────────────────────────────────────────────────────────────

func TestMemoryStore_Reserve_NewKey_Granted(t *testing.T) {
	t.Parallel()
	s := newMemStore()

	ok, err := s.Reserve(context.Background(), "k1", time.Minute)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !ok {
		t.Error("Reserve on new key: want granted=true, got false")
	}
}

func TestMemoryStore_Reserve_ExistingKey_Denied(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "k2", time.Minute)

	ok, err := s.Reserve(ctx, "k2", time.Minute)
	if err != nil {
		t.Fatalf("second Reserve: %v", err)
	}
	if ok {
		t.Error("second Reserve on existing key: want granted=false, got true")
	}
}

func TestMemoryStore_Reserve_ExpiredKey_Granted(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "k3", time.Nanosecond)
	time.Sleep(time.Millisecond)

	ok, err := s.Reserve(ctx, "k3", time.Minute)
	if err != nil {
		t.Fatalf("Reserve after expiry: %v", err)
	}
	if !ok {
		t.Error("Reserve after expiry: want granted=true, got false")
	}
}

// ── Load after Reserve ────────────────────────────────────────────────────────

func TestMemoryStore_LoadAfterReserve_InFlightState(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "k4", time.Minute)

	e, found, err := s.Load(ctx, "k4")
	if err != nil || !found {
		t.Fatalf("Load after Reserve: found=%v err=%v", found, err)
	}
	if e.State != idempotency.InFlight {
		t.Errorf("state after Reserve: want InFlight, got %v", e.State)
	}
}

// ── Commit ────────────────────────────────────────────────────────────────────

func TestMemoryStore_Commit_SetsCommittedEntry(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "k5", time.Minute)

	want := committedEntry()
	if err := s.Commit(ctx, "k5", want, time.Minute); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, found, err := s.Load(ctx, "k5")
	if err != nil || !found {
		t.Fatalf("Load after Commit: found=%v err=%v", found, err)
	}
	if got.State != idempotency.Committed {
		t.Errorf("state: want Committed, got %v", got.State)
	}
	if got.StatusCode != want.StatusCode {
		t.Errorf("status: want %d, got %d", want.StatusCode, got.StatusCode)
	}
	if string(got.Body) != string(want.Body) {
		t.Errorf("body: want %q, got %q", want.Body, got.Body)
	}
}

func TestMemoryStore_Commit_ExpiredAfterTTL(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "k6", time.Minute)
	_ = s.Commit(ctx, "k6", committedEntry(), time.Nanosecond)

	time.Sleep(time.Millisecond)

	_, found, err := s.Load(ctx, "k6")
	if err != nil {
		t.Fatalf("Load after TTL: %v", err)
	}
	if found {
		t.Error("committed entry must expire after TTL")
	}
}

// ── Release ───────────────────────────────────────────────────────────────────

func TestMemoryStore_Release_RemovesKey(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	_, _ = s.Reserve(ctx, "k7", time.Minute)
	if err := s.Release(ctx, "k7"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	_, found, err := s.Load(ctx, "k7")
	if err != nil || found {
		t.Errorf("Load after Release: found=%v err=%v (want found=false)", found, err)
	}
}

func TestMemoryStore_Release_UnknownKey_IsNoop(t *testing.T) {
	t.Parallel()
	s := newMemStore()

	if err := s.Release(context.Background(), "does-not-exist"); err != nil {
		t.Errorf("Release of unknown key must not error; got: %v", err)
	}
}

// ── Concurrent safety ─────────────────────────────────────────────────────────

// Multiple goroutines racing to Reserve the same key: exactly one must win.
func TestMemoryStore_Reserve_Concurrent_OnlyOneWins(t *testing.T) {
	t.Parallel()
	s := newMemStore()
	ctx := context.Background()

	const n = 50
	wins := make([]bool, n)
	var wg sync.WaitGroup

	for i := range wins {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ok, err := s.Reserve(ctx, "race-key", time.Minute)
			if err != nil {
				t.Errorf("Reserve[%d]: %v", idx, err)
				return
			}
			wins[idx] = ok
		}(i)
	}
	wg.Wait()

	var count int
	for _, w := range wins {
		if w {
			count++
		}
	}
	if count != 1 {
		t.Errorf("concurrent Reserve: want exactly 1 winner, got %d", count)
	}
}
