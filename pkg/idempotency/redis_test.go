package idempotency_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

// newTestStore starts an in-process Redis (miniredis) and returns a RedisStore
// connected to it. The miniredis server is stopped automatically when t ends.
func newTestStore(t *testing.T) (*idempotency.RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return idempotency.NewRedisStore(client), mr
}

func committedEntry() idempotency.Entry {
	return idempotency.Entry{
		State:      idempotency.Committed,
		StatusCode: http.StatusCreated,
		Headers:    http.Header{"Content-Type": {"application/json"}, "X-Resource-Id": {"42"}},
		Body:       []byte(`{"id":42}`),
	}
}

// ── Load ──────────────────────────────────────────────────────────────────────

func TestRedisStore_Load_UnknownKey_ReturnsFalse(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	e, found, err := s.Load(context.Background(), "no-such-key")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if found {
		t.Errorf("Load on unknown key: want found=false, got found=true (entry=%+v)", e)
	}
}

func TestRedisStore_Load_CorruptedValue_ReturnsFalse(t *testing.T) {
	t.Parallel()
	s, mr := newTestStore(t)

	// Write garbage directly into Redis to simulate schema corruption.
	mr.Set("bad-key", "not-json")

	_, found, err := s.Load(context.Background(), "bad-key")
	if err != nil {
		t.Fatalf("Load on corrupted key: unexpected error: %v", err)
	}
	if found {
		t.Error("Load on corrupted key: want found=false")
	}
}

// ── Reserve ───────────────────────────────────────────────────────────────────

func TestRedisStore_Reserve_NewKey_Granted(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	ok, err := s.Reserve(context.Background(), "k1", time.Minute)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !ok {
		t.Error("Reserve on new key: want granted=true, got false")
	}
}

func TestRedisStore_Reserve_ExistingKey_Denied(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	_, _ = s.Reserve(context.Background(), "k2", time.Minute)

	ok, err := s.Reserve(context.Background(), "k2", time.Minute)
	if err != nil {
		t.Fatalf("second Reserve: %v", err)
	}
	if ok {
		t.Error("second Reserve on existing key: want granted=false, got true")
	}
}

func TestRedisStore_Reserve_ExpiredKey_Granted(t *testing.T) {
	t.Parallel()
	s, mr := newTestStore(t)

	_, _ = s.Reserve(context.Background(), "k3", time.Millisecond)
	mr.FastForward(time.Second) // expire the key

	ok, err := s.Reserve(context.Background(), "k3", time.Minute)
	if err != nil {
		t.Fatalf("Reserve after expiry: %v", err)
	}
	if !ok {
		t.Error("Reserve after expiry: want granted=true, got false")
	}
}

// ── Load after Reserve ────────────────────────────────────────────────────────

func TestRedisStore_LoadAfterReserve_InFlightState(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	_, _ = s.Reserve(context.Background(), "k4", time.Minute)

	e, found, err := s.Load(context.Background(), "k4")
	if err != nil || !found {
		t.Fatalf("Load after Reserve: found=%v err=%v", found, err)
	}
	if e.State != idempotency.InFlight {
		t.Errorf("state after Reserve: want InFlight, got %v", e.State)
	}
}

// ── Commit ────────────────────────────────────────────────────────────────────

func TestRedisStore_Commit_SetsCommittedEntry(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

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
	if got.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("header Content-Type: want application/json, got %q", got.Headers.Get("Content-Type"))
	}
	if got.Headers.Get("X-Resource-Id") != "42" {
		t.Errorf("header X-Resource-Id: want 42, got %q", got.Headers.Get("X-Resource-Id"))
	}
}

func TestRedisStore_Commit_ExpiredAfterTTL(t *testing.T) {
	t.Parallel()
	s, mr := newTestStore(t)

	ctx := context.Background()
	_, _ = s.Reserve(ctx, "k6", time.Minute)
	_ = s.Commit(ctx, "k6", committedEntry(), time.Millisecond)

	mr.FastForward(time.Second)

	_, found, err := s.Load(ctx, "k6")
	if err != nil {
		t.Fatalf("Load after TTL: %v", err)
	}
	if found {
		t.Error("committed entry must expire after TTL")
	}
}

// ── Release ───────────────────────────────────────────────────────────────────

func TestRedisStore_Release_RemovesKey(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

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

func TestRedisStore_Release_UnknownKey_IsNoop(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	if err := s.Release(context.Background(), "does-not-exist"); err != nil {
		t.Errorf("Release of unknown key must not error; got: %v", err)
	}
}

// ── Error propagation ─────────────────────────────────────────────────────────

func TestRedisStore_Load_CancelledContext_ReturnsError(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, _, err := s.Load(ctx, "any-key")
	if err == nil {
		t.Error("Load with cancelled context: want error, got nil")
	}
}

func TestRedisStore_Reserve_CancelledContext_ReturnsError(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Reserve(ctx, "any-key", time.Minute)
	if err == nil {
		t.Error("Reserve with cancelled context: want error, got nil")
	}
}

func TestRedisStore_Commit_CancelledContext_ReturnsError(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Commit(ctx, "any-key", committedEntry(), time.Minute); err == nil {
		t.Error("Commit with cancelled context: want error, got nil")
	}
}

func TestRedisStore_Release_CancelledContext_ReturnsError(t *testing.T) {
	t.Parallel()
	s, _ := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Release(ctx, "any-key"); err == nil {
		t.Error("Release with cancelled context: want error, got nil")
	}
}

// ── Cross-instance isolation ──────────────────────────────────────────────────

// Two RedisStore instances sharing the same Redis backend must see each
// other's reservations — the core property that makes horizontal scaling safe.
func TestRedisStore_TwoInstances_SameRedis_ShareReservations(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	newClient := func() *redis.Client {
		c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = c.Close() })
		return c
	}

	s1 := idempotency.NewRedisStore(newClient())
	s2 := idempotency.NewRedisStore(newClient())

	ctx := context.Background()
	ok, err := s1.Reserve(ctx, "shared-key", time.Minute)
	if err != nil || !ok {
		t.Fatalf("s1.Reserve: ok=%v err=%v", ok, err)
	}

	// s2 (simulating a different replica) must not be able to claim the same key.
	ok2, err := s2.Reserve(ctx, "shared-key", time.Minute)
	if err != nil || ok2 {
		t.Errorf("s2.Reserve on already-held key: want (false, nil), got (%v, %v)", ok2, err)
	}
}
