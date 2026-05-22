package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// stubSystemParamRepoBench is a minimal, allocation-free stub used exclusively
// by benchmarks. It avoids the closures used in the table-driven unit tests
// so benchmark results reflect only the service code under measurement.
type stubSystemParamRepoBench struct {
	p *domain.SystemParam
}

func (r *stubSystemParamRepoBench) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return r.p, nil
}
func (r *stubSystemParamRepoBench) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return []*domain.SystemParam{r.p}, nil
}
func (r *stubSystemParamRepoBench) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return []*domain.SystemParam{r.p}, nil
}
func (r *stubSystemParamRepoBench) Set(_ context.Context, key, value string, _ int) (*domain.SystemParam, error) {
	return &domain.SystemParam{Key: key, Value: value}, nil
}
func (r *stubSystemParamRepoBench) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (r *stubSystemParamRepoBench) ResetToDefault(_ context.Context, _ string) (*domain.SystemParam, error) {
	return r.p, nil
}

// seededParam returns a SystemParam whose Type is string so validateParamValue
// is a trivial no-op, keeping Set benchmarks focused on service overhead.
func seededParam(key, value string) *domain.SystemParam {
	return &domain.SystemParam{
		Key:       key,
		Value:     value,
		Type:      domain.SystemParamTypeString,
		IsRuntime: true,
	}
}

// BenchmarkSystemParamGet_CacheHit measures the cost of reading a warm cache
// entry under concurrent load. This is the hot path executed on every request
// that calls GetInt / GetString.
func BenchmarkSystemParamGet_CacheHit(b *testing.B) {
	repo := &stubSystemParamRepoBench{p: seededParam("k", "v")}
	svc := NewSystemParamService(repo, nil, zap.NewNop())
	ctx := context.Background()

	// Warm the cache with a single read before the timed loop.
	_, _ = svc.Get(ctx, "k")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := svc.Get(ctx, "k"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSystemParamGet_CacheMiss measures the cost of a cache miss: the
// service calls into the repository and populates the cache for future reads.
// We evict after every read to keep the miss path exercised throughout the run.
func BenchmarkSystemParamGet_CacheMiss(b *testing.B) {
	repo := &stubSystemParamRepoBench{p: seededParam("k", "v")}
	svc := NewSystemParamService(repo, nil, zap.NewNop()).(*systemParamService)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.evict("k")
		if _, err := svc.Get(ctx, "k"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSystemParamSet measures write throughput including type validation,
// cache eviction, and hook dispatch. Each iteration is a distinct key to avoid
// cache interactions between goroutines.
func BenchmarkSystemParamSet(b *testing.B) {
	repo := &stubSystemParamRepoBench{p: seededParam("k", "v")}
	svc := NewSystemParamService(repo, nil, zap.NewNop())
	ctx := context.Background()

	// Pre-seed the cache so validateAndGetExisting reads from cache,
	// not the repo, matching the production path.
	_, _ = svc.Get(ctx, "k")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Set(ctx, "k", "new", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSystemParamGet_HighConcurrency stress-tests the RWMutex guard around
// the in-memory cache. 100 concurrent goroutines hammer the same key.
func BenchmarkSystemParamGet_HighConcurrency(b *testing.B) {
	repo := &stubSystemParamRepoBench{p: seededParam("shared", "value")}
	svc := NewSystemParamService(repo, nil, zap.NewNop())
	ctx := context.Background()
	_, _ = svc.Get(ctx, "shared") // warm

	const goroutines = 100
	b.ResetTimer()
	b.SetParallelism(goroutines)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := svc.Get(ctx, "shared"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSystemParamBulkSet measures the cost of a 10-key atomic bulk write.
// This covers the validation loop, the repository call, cache eviction for all
// keys, and per-key hook dispatch.
func BenchmarkSystemParamBulkSet(b *testing.B) {
	params := make(map[string]string, 10)
	for i := 0; i < 10; i++ {
		params[fmt.Sprintf("k%d", i)] = "v"
	}

	repo := &stubSystemParamRepoBench{p: seededParam("k", "v")}
	svc := NewSystemParamService(repo, nil, zap.NewNop())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := svc.BulkSet(ctx, params, 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMutationHookDispatch measures the overhead of invoking N registered
// hooks for a single key. The hook itself is a no-op; the benchmark isolates the
// dispatch loop cost.
func BenchmarkMutationHookDispatch(b *testing.B) {
	repo := &stubSystemParamRepoBench{p: seededParam("k", "v")}
	svc := NewSystemParamService(repo, nil, zap.NewNop())
	ctx := context.Background()
	_, _ = svc.Get(ctx, "k") // warm

	mhr := svc.(MutationHookRegisterer)
	var mu sync.Mutex
	calls := 0
	for i := 0; i < 10; i++ {
		mhr.RegisterMutationHook("k", func(_ context.Context) {
			mu.Lock()
			calls++
			mu.Unlock()
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Set(ctx, "k", "v", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetInt measures GetInt throughput — the primary API consumed by
// handler middleware that reads runtime params on every request.
func BenchmarkGetInt(b *testing.B) {
	p := seededParam("scoring.exact_score", "10")
	p.Type = domain.SystemParamTypeInt
	repo := &stubSystemParamRepoBench{p: p}
	svc := NewSystemParamService(repo, nil, zap.NewNop())
	ctx := context.Background()
	_ = svc.GetInt(ctx, "scoring.exact_score", 0) // warm

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = svc.GetInt(ctx, "scoring.exact_score", 0)
		}
	})
}

// BenchmarkCacheTTLEviction measures Get throughput as cache entries expire
// after a very short TTL. This exercises the expiry-check branch.
func BenchmarkCacheTTLEviction(b *testing.B) {
	repo := &stubSystemParamRepoBench{p: seededParam("k", "v")}
	svc := NewSystemParamService(repo, nil, zap.NewNop(),
		withTTL(1*time.Nanosecond),
	)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Get(ctx, "k"); err != nil {
			b.Fatal(err)
		}
	}
}

// withTTL is a test-only SystemParamServiceOption that overrides both TTLs.
func withTTL(ttl time.Duration) SystemParamServiceOption {
	return func(s *systemParamService) {
		s.ttl = ttl
		s.runtimeTTL = ttl
	}
}
