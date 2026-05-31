//go:build integration

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestConcurrentReadinessProbes fires concurrency GET /health/ready requests
// simultaneously to validate that the DB connection pool handles concurrent
// pings without exhaustion, 500 errors, or deadlocks.
//
// The test uses a pool with MaxOpenConns=5 (set by trySetupE2EDB), which is
// intentionally smaller than production's 25.  Running 50 goroutines against
// a 5-connection pool exercises pool queuing: if queuing is broken, requests
// time out or return 500 due to "too many connections" errors.
//
// This guards against the failure mode described in docs/capacity.md where a
// burst of requests — such as Fly.io waking a machine and immediately routing
// hard_limit=250 concurrent requests — exhausts the pool.
func TestConcurrentReadinessProbes(t *testing.T) {
	skipIfNoE2EDB(t)

	// 50 goroutines against a 5-connection pool (10× oversubscription).
	// Production uses MaxOpenConns=25 with hard_limit=250 (also 10×), so
	// this ratio faithfully represents the production stress level.
	const concurrency = 50

	h := newE2EServer(t, "").Routes(context.Background())

	codes := make(chan int, concurrency)
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			h.ServeHTTP(rec, req)
			codes <- rec.Code
		}()
	}

	wg.Wait()
	close(codes)

	var failures int
	for code := range codes {
		if code != http.StatusOK {
			t.Errorf("concurrent /health/ready: got %d, want 200", code)
			failures++
		}
	}
	if failures > 0 {
		t.Errorf("%d/%d concurrent readiness probes failed — check DB pool exhaustion (MaxOpenConns=%d, goroutines=%d)",
			failures, concurrency, 5, concurrency)
	}
}
