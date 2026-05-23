package cache_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

// ── Get ───────────────────────────────────────────────────────────────────────

func TestMemoryStore_Get_MissingKey_ReturnsErrCacheMiss(t *testing.T) {
	s := cache.NewMemoryStore()
	var dest string
	if err := s.Get(context.Background(), "no-such-key", &dest); !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestMemoryStore_Get_ValidKey_DeserializesValue(t *testing.T) {
	s := cache.NewMemoryStore()
	if err := s.Set(context.Background(), "k", "hello", time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var dest string
	if err := s.Get(context.Background(), "k", &dest); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if dest != "hello" {
		t.Errorf("got %q, want %q", dest, "hello")
	}
}

func TestMemoryStore_Get_CorruptedJSON_ReturnsErrCacheMiss(t *testing.T) {
	// Use resilient-store stub pattern: create an instance via reflect tricks
	// isn't available to _test packages. We rely on Set always producing valid
	// JSON, so this test verifies the miss path for type mismatch instead.
	s := cache.NewMemoryStore()
	if err := s.Set(context.Background(), "k", 42, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Unmarshal int into a struct → type mismatch → ErrCacheMiss
	var dest struct{ Name string }
	if err := s.Get(context.Background(), "k", &dest); !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss on type mismatch, got %v", err)
	}
}

// ── Set ───────────────────────────────────────────────────────────────────────

func TestMemoryStore_Set_StoresAndRetrievesStruct(t *testing.T) {
	type payload struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	s := cache.NewMemoryStore()
	in := payload{ID: 7, Name: "Brazil"}
	if err := s.Set(context.Background(), "k", in, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var out payload
	if err := s.Get(context.Background(), "k", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.ID != 7 || out.Name != "Brazil" {
		t.Errorf("expected {7 Brazil}, got %+v", out)
	}
}

func TestMemoryStore_Set_ZeroTTL_EntryPersists(t *testing.T) {
	s := cache.NewMemoryStore()
	if err := s.Set(context.Background(), "k", "v", 0); err != nil {
		t.Fatalf("Set with zero TTL: %v", err)
	}
	var dest string
	if err := s.Get(context.Background(), "k", &dest); err != nil {
		t.Fatalf("Get after Set with zero TTL: %v", err)
	}
}

func TestMemoryStore_Set_OverwritesExistingKey(t *testing.T) {
	s := cache.NewMemoryStore()
	_ = s.Set(context.Background(), "k", "first", 0)
	_ = s.Set(context.Background(), "k", "second", 0)
	var dest string
	_ = s.Get(context.Background(), "k", &dest)
	if dest != "second" {
		t.Errorf("expected second write to win, got %q", dest)
	}
}

func TestMemoryStore_Set_UnmarshalableValue_ReturnsError(t *testing.T) {
	s := cache.NewMemoryStore()
	// channels cannot be JSON-marshalled; Set must propagate the marshal error.
	err := s.Set(context.Background(), "k", make(chan int), 0)
	if err == nil {
		t.Error("expected error marshalling channel, got nil")
	}
	// The key must not have been written.
	var dest int
	if getErr := s.Get(context.Background(), "k", &dest); getErr == nil {
		t.Error("expected ErrCacheMiss for key that failed to set, got nil")
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestMemoryStore_Delete_ExistingKey_RemovesIt(t *testing.T) {
	s := cache.NewMemoryStore()
	_ = s.Set(context.Background(), "k", "v", 0)
	if err := s.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var dest string
	if err := s.Get(context.Background(), "k", &dest); !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss after Delete, got %v", err)
	}
}

func TestMemoryStore_Delete_NoKeys_ReturnsNil(t *testing.T) {
	s := cache.NewMemoryStore()
	if err := s.Delete(context.Background()); err != nil {
		t.Errorf("expected nil for empty Delete, got %v", err)
	}
}

func TestMemoryStore_Delete_MissingKey_ReturnsNil(t *testing.T) {
	s := cache.NewMemoryStore()
	if err := s.Delete(context.Background(), "ghost"); err != nil {
		t.Errorf("expected nil when deleting non-existent key, got %v", err)
	}
}

func TestMemoryStore_Delete_MultipleKeys_RemovesAll(t *testing.T) {
	s := cache.NewMemoryStore()
	for _, k := range []string{"k1", "k2", "k3"} {
		_ = s.Set(context.Background(), k, k, 0)
	}
	if err := s.Delete(context.Background(), "k1", "k2", "k3"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Len() != 0 {
		t.Errorf("expected store to be empty after deleting all keys, got Len=%d", s.Len())
	}
}

// ── FlushByPrefix ─────────────────────────────────────────────────────────────

func TestMemoryStore_FlushByPrefix_DeletesMatchingKeys(t *testing.T) {
	s := cache.NewMemoryStore()
	ctx := context.Background()
	for _, k := range []string{"lb:1", "lb:2", "other:3"} {
		_ = s.Set(ctx, k, "v", 0)
	}
	if err := s.FlushByPrefix(ctx, "lb:"); err != nil {
		t.Fatalf("FlushByPrefix: %v", err)
	}
	for _, k := range []string{"lb:1", "lb:2"} {
		var dest string
		if err := s.Get(ctx, k, &dest); !errors.Is(err, cache.ErrCacheMiss) {
			t.Errorf("key %q should have been flushed, got %v", k, err)
		}
	}
	var dest string
	if err := s.Get(ctx, "other:3", &dest); err != nil {
		t.Errorf("key outside prefix was unexpectedly deleted: %v", err)
	}
}

func TestMemoryStore_FlushByPrefix_NoMatchingKeys_ReturnsNil(t *testing.T) {
	s := cache.NewMemoryStore()
	_ = s.Set(context.Background(), "other:1", "v", 0)
	if err := s.FlushByPrefix(context.Background(), "lb:"); err != nil {
		t.Errorf("expected nil for prefix with no matches, got %v", err)
	}
	if s.Len() != 1 {
		t.Errorf("non-matching key should not be deleted")
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestMemoryStore_ConcurrentAccess_DoesNotRace(t *testing.T) {
	s := cache.NewMemoryStore()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			_ = s.Set(ctx, "key", i, 0)
		}(i)
		go func() {
			defer wg.Done()
			var dest int
			_ = s.Get(ctx, "key", &dest)
		}()
		go func() {
			defer wg.Done()
			_ = s.Delete(ctx, "key")
		}()
	}
	wg.Wait()
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestMemoryStore_ImplementsStore(t *testing.T) {
	var _ cache.Store = (*cache.MemoryStore)(nil)
}

func TestMemoryStore_ImplementsPrefixFlusher(t *testing.T) {
	var _ cache.PrefixFlusher = (*cache.MemoryStore)(nil)
}
