package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// newBenchServer constructs the minimal server needed for handler benchmarks.
// db=nil so Routes() registers the db-unavailable catch-all — infrastructure
// endpoints (/health, /swagger) remain fully functional.
func newBenchServer(b *testing.B) *api.Server {
	b.Helper()
	return api.New(nil, &config.Config{}, zaptest.NewLogger(b), messaging.NewInMemoryBus(nil), nil, nil)
}

// BenchmarkHandlerHealth measures the end-to-end throughput of GET /health
// through the full middleware stack (RequestID, RealIP, Recovery, Logger, CORS).
// This is the performance baseline for the simplest handler in the system.
func BenchmarkHandlerHealth(b *testing.B) {
	srv := newBenchServer(b)
	handler := srv.Routes(context.Background())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				b.Fatalf("unexpected status: %d", w.Code)
			}
		}
	})
}

// BenchmarkHandlerHealth_Sequential measures single-goroutine throughput for
// /health. Pairing with BenchmarkHandlerHealth (parallel) reveals whether
// the middleware stack has lock contention under concurrency.
func BenchmarkHandlerHealth_Sequential(b *testing.B) {
	srv := newBenchServer(b)
	handler := srv.Routes(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkHandlerRouting measures the router's path-matching overhead for a
// deeply-nested route that exercises multiple URL parameters and middleware
// layers. With db=nil, /api/v1/* returns 503 immediately — the benchmark
// isolates chi routing cost, not handler logic.
func BenchmarkHandlerRouting(b *testing.B) {
	srv := newBenchServer(b)
	handler := srv.Routes(context.Background())

	paths := []string{
		"/api/v1/matches",
		"/api/v1/predictions",
		"/api/v1/groups/1/members",
		"/api/v1/admin/system-params",
		"/api/v1/admin/system-params/scoring.exact_score/history",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := paths[i%len(paths)]
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			i++
		}
	})
}

// BenchmarkHandlerNotFound measures the 404 path — important because
// misrouted requests are a common attack vector that could saturate the server.
func BenchmarkHandlerNotFound(b *testing.B) {
	srv := newBenchServer(b)
	handler := srv.Routes(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/nonexistent/deep/path/12345", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}
	})
}
