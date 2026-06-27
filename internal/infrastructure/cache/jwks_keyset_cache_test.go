package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestJWKSKeysetCache_SaveAndLoad_ReturnsSamePayload(t *testing.T) {
	rc := newTestRedisClient(t)
	c := cache.NewJWKSKeysetCache(rc)
	ctx := context.Background()

	payload := []byte(`{"keys":[{"kty":"RSA","kid":"abc"}]}`)
	if err := c.Save(ctx, payload, time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := c.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("Load returned %q; want %q", got, payload)
	}
}

func TestJWKSKeysetCache_Load_CacheMiss_ReturnsNilNil(t *testing.T) {
	rc := newTestRedisClient(t)
	c := cache.NewJWKSKeysetCache(rc)

	got, err := c.Load(context.Background())
	if err != nil {
		t.Fatalf("Load on empty cache: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("Load on empty cache: expected nil payload, got %q", got)
	}
}

func TestJWKSKeysetCache_Save_TTLExpiry_SubsequentLoadReturnsMiss(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := cache.NewJWKSKeysetCache(rc)
	ctx := context.Background()

	if err := c.Save(ctx, []byte(`{"keys":[]}`), time.Second); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Advance miniredis clock past the TTL so the key expires.
	mr.FastForward(2 * time.Second)

	got, err := c.Load(ctx)
	if err != nil {
		t.Fatalf("Load after expiry: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("Load after expiry: expected nil (cache miss), got %q", got)
	}
}
