package dsem_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/pkg/dsem"
)

func newTestClient(t *testing.T) (*miniredis.Miniredis, redis.Cmdable) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rc.Close() })
	return mr, rc
}

func TestRedisSemaphore_AcquireRelease(t *testing.T) {
	_, rc := newTestClient(t)
	sem := dsem.New(rc, "test:sem", 2, 10*time.Second)

	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	sem.Release()
	// After one release the limit drops to 1; a third acquire must succeed.
	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatalf("third acquire after release: %v", err)
	}
	sem.Release()
	sem.Release()
}

func TestRedisSemaphore_LimitEnforced(t *testing.T) {
	_, rc := newTestClient(t)
	sem := dsem.New(rc, "test:sem:limit", 2, 10*time.Second)

	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Third acquire must block; cancel immediately to verify it returns ctx.Err().
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := sem.Acquire(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context; got nil")
	}

	sem.Release()
	sem.Release()
}

func TestRedisSemaphore_ConcurrentAcquireRelease(t *testing.T) {
	_, rc := newTestClient(t)
	const limit = 3
	const goroutines = 10
	sem := dsem.New(rc, "test:sem:concurrent", limit, 30*time.Second)

	var active atomic.Int64
	var maxObserved atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sem.Acquire(context.Background()); err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			cur := active.Add(1)
			// Track the maximum concurrent holders.
			for {
				old := maxObserved.Load()
				if cur <= old || maxObserved.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
			sem.Release()
		}()
	}
	wg.Wait()

	if max := maxObserved.Load(); max > limit {
		t.Errorf("concurrent holders exceeded limit: got %d; want ≤ %d", max, limit)
	}
}

func TestRedisSemaphore_DoubleRelease_NoNegative(t *testing.T) {
	mr, rc := newTestClient(t)
	sem := dsem.New(rc, "test:sem:double", 2, 10*time.Second)

	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	sem.Release()
	sem.Release() // double-release must not drive the counter negative

	val, err := redis.NewClient(&redis.Options{Addr: mr.Addr()}).Get(context.Background(), "test:sem:double").Int64()
	if err == redis.Nil {
		return // key expired or already zero; acceptable
	}
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if val < 0 {
		t.Errorf("counter went negative after double-release: %d", val)
	}
}

func TestRedisSemaphore_ContextCancelled_ReturnsImmediately(t *testing.T) {
	_, rc := newTestClient(t)
	sem := dsem.New(rc, "test:sem:cancel", 1, 10*time.Second)

	// Fill the single slot.
	if err := sem.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	start := time.Now()
	err := sem.Acquire(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from pre-cancelled context; got nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("cancelled acquire took too long: %v", elapsed)
	}

	sem.Release()
}
