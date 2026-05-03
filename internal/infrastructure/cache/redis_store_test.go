package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

const (
	testValue       = "hello world"
	fmtSetErr       = "Set: %v"
	msgRedisFailErr = "expected error on Redis failure, got nil"
	keyToDelete     = "to-delete"
)

// newTestStore starts a miniredis server and returns a RedisStore connected to
// it together with the underlying miniredis handle for low-level manipulation.
func newTestStore(t *testing.T) (*miniredis.Miniredis, *cache.RedisStore) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return mr, cache.NewRedisStore(rc)
}

// ── ErrCacheMiss ──────────────────────────────────────────────────────────────

func TestErrCacheMiss_Error_ReturnsCacheMiss(t *testing.T) {
	if cache.ErrCacheMiss.Error() != "cache miss" {
		t.Errorf("expected ErrCacheMiss.Error()=%q, got %q", "cache miss", cache.ErrCacheMiss.Error())
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestRedisStore_Get_MissingKey_ReturnsErrCacheMiss(t *testing.T) {
	_, st := newTestStore(t)
	var dest string
	err := st.Get(context.Background(), "no-such-key", &dest)
	if !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss for missing key, got %v", err)
	}
}

func TestRedisStore_Get_ValidKey_DeserializesValue(t *testing.T) {
	_, st := newTestStore(t)
	// Seed via Set so the stored format matches exactly.
	if err := st.Set(context.Background(), "mykey", testValue, time.Minute); err != nil {
		t.Fatalf(fmtSetErr, err)
	}
	var dest string
	if err := st.Get(context.Background(), "mykey", &dest); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if dest != testValue {
		t.Errorf("expected %q, got %q", testValue, dest)
	}
}

func TestRedisStore_Get_CorruptedJSON_ReturnsErrCacheMiss(t *testing.T) {
	mr, st := newTestStore(t)
	// Manually write invalid JSON bytes - bypasses Set's marshal step.
	mr.Set("bad-json-key", "not-json")

	var dest map[string]string
	err := st.Get(context.Background(), "bad-json-key", &dest)
	if !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss for corrupted JSON, got %v", err)
	}
}

func TestRedisStore_Get_RedisError_ReturnsWrappedError(t *testing.T) {
	mr, st := newTestStore(t)
	mr.Close() // force all Redis commands to fail

	var dest string
	err := st.Get(context.Background(), "any-key", &dest)
	if err == nil {
		t.Fatal(msgRedisFailErr)
	}
	if errors.Is(err, cache.ErrCacheMiss) {
		t.Error("Redis network error should not be returned as ErrCacheMiss")
	}
}

// ── Set ───────────────────────────────────────────────────────────────────────

func TestRedisStore_Set_StoresAndRetrievesStruct(t *testing.T) {
	_, st := newTestStore(t)
	type payload struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	in := payload{ID: 7, Name: "Brazil"}
	if err := st.Set(context.Background(), "payload-key", in, time.Minute); err != nil {
		t.Fatalf(fmtSetErr, err)
	}
	var out payload
	if err := st.Get(context.Background(), "payload-key", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.ID != 7 || out.Name != "Brazil" {
		t.Errorf("expected {7 Brazil}, got %+v", out)
	}
}

func TestRedisStore_Set_WithZeroTTL_StoresForever(t *testing.T) {
	_, st := newTestStore(t)
	if err := st.Set(context.Background(), "no-ttl", "value", 0); err != nil {
		t.Fatalf("Set with zero TTL: %v", err)
	}
	var dest string
	if err := st.Get(context.Background(), "no-ttl", &dest); err != nil {
		t.Fatalf("Get after Set with zero TTL: %v", err)
	}
}

func TestRedisStore_Set_RedisError_ReturnsError(t *testing.T) {
	mr, st := newTestStore(t)
	mr.Close()

	err := st.Set(context.Background(), "key", "val", time.Minute)
	if err == nil {
		t.Fatal(msgRedisFailErr)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestRedisStore_Delete_ExistingKey_RemovesIt(t *testing.T) {
	_, st := newTestStore(t)
	if err := st.Set(context.Background(), keyToDelete, "value", time.Minute); err != nil {
		t.Fatalf(fmtSetErr, err)
	}
	if err := st.Delete(context.Background(), keyToDelete); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var dest string
	if err := st.Get(context.Background(), keyToDelete, &dest); !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss after Delete, got %v", err)
	}
}

func TestRedisStore_Delete_NoKeys_ReturnsNil(t *testing.T) {
	_, st := newTestStore(t)
	if err := st.Delete(context.Background()); err != nil {
		t.Errorf("expected nil for empty Delete, got %v", err)
	}
}

func TestRedisStore_Delete_MissingKey_ReturnsNil(t *testing.T) {
	_, st := newTestStore(t)
	if err := st.Delete(context.Background(), "ghost-key"); err != nil {
		t.Errorf("expected nil when deleting non-existent key, got %v", err)
	}
}

func TestRedisStore_Delete_MultipleKeys_RemovesAll(t *testing.T) {
	_, st := newTestStore(t)
	for _, k := range []string{"k1", "k2", "k3"} {
		if err := st.Set(context.Background(), k, k, time.Minute); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}
	if err := st.Delete(context.Background(), "k1", "k2", "k3"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, k := range []string{"k1", "k2", "k3"} {
		var dest string
		if err := st.Get(context.Background(), k, &dest); !errors.Is(err, cache.ErrCacheMiss) {
			t.Errorf("expected ErrCacheMiss for %q after Delete, got %v", k, err)
		}
	}
}

func TestRedisStore_Delete_RedisError_ReturnsError(t *testing.T) {
	mr, st := newTestStore(t)
	mr.Close()

	err := st.Delete(context.Background(), "key")
	if err == nil {
		t.Fatal(msgRedisFailErr)
	}
}

// ── FlushByPrefix ─────────────────────────────────────────────────────────────

func TestRedisStore_FlushByPrefix_NoMatchingKeys_ReturnsNil(t *testing.T) {
	_, st := newTestStore(t)
	ctx := context.Background()

	// Seed a key that must NOT be deleted.
	if err := st.Set(ctx, "other:key1", "v", time.Minute); err != nil {
		t.Fatalf(fmtSetErr, err)
	}

	if err := st.FlushByPrefix(ctx, "leaderboard:"); err != nil {
		t.Fatalf("FlushByPrefix on empty prefix: %v", err)
	}

	// Unrelated key must survive.
	var dest string
	if err := st.Get(ctx, "other:key1", &dest); err != nil {
		t.Errorf("key outside prefix was unexpectedly deleted: %v", err)
	}
}

func TestRedisStore_FlushByPrefix_MatchingKeys_DeletesThem(t *testing.T) {
	_, st := newTestStore(t)
	ctx := context.Background()

	// Seed matching and non-matching keys.
	for _, k := range []string{"leaderboard:1", "leaderboard:2"} {
		if err := st.Set(ctx, k, "v", time.Minute); err != nil {
			t.Fatalf(fmtSetErr, err)
		}
	}
	if err := st.Set(ctx, "other:key", "v", time.Minute); err != nil {
		t.Fatalf(fmtSetErr, err)
	}

	if err := st.FlushByPrefix(ctx, "leaderboard:"); err != nil {
		t.Fatalf("FlushByPrefix: %v", err)
	}

	for _, k := range []string{"leaderboard:1", "leaderboard:2"} {
		var dest string
		if err := st.Get(ctx, k, &dest); !errors.Is(err, cache.ErrCacheMiss) {
			t.Errorf("key %q: expected ErrCacheMiss after flush, got %v", k, err)
		}
	}

	// Key outside the prefix must survive.
	var dest string
	if err := st.Get(ctx, "other:key", &dest); err != nil {
		t.Errorf("key outside prefix was unexpectedly deleted: %v", err)
	}
}

func TestRedisStore_FlushByPrefix_ScanError_ReturnsWrappedError(t *testing.T) {
	mr, st := newTestStore(t)
	mr.Close() // force all Redis commands to fail

	if err := st.FlushByPrefix(context.Background(), "leaderboard:"); err == nil {
		t.Fatal("expected error when Redis is unavailable, got nil")
	}
}
