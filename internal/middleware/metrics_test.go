package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// spyFlusher implements http.ResponseWriter + http.Flusher so we can assert
// that the metrics middleware forwards Flush calls to the underlying writer.
type spyFlusher struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *spyFlusher) Flush() { f.flushed = true }

// opaqueWriter wraps an http.ResponseWriter but does NOT expose http.Flusher,
// verifying that the middleware's Flush path degrades gracefully.
type opaqueWriter struct{ http.ResponseWriter }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newMeterProvider(t *testing.T) (*sdkmetric.MeterProvider, sdkmetric.Reader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return mp, reader
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestNewMetrics_CallsNextHandler verifies that the middleware is transparent:
// the inner handler runs and its response reaches the client.
func TestNewMetrics_CallsNextHandler(t *testing.T) {
	t.Parallel()
	mp, _ := newMeterProvider(t)

	called := false
	h := middleware.NewMetrics(mp.Meter("test"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusAccepted)
		}),
	)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/test", nil))

	if !called {
		t.Error("expected inner handler to be called")
	}
	if rr.Code != http.StatusAccepted {
		t.Errorf("status: got %d; want %d", rr.Code, http.StatusAccepted)
	}
}

// TestNewMetrics_RecordsDuration verifies that after a request the histogram
// instrument appears in the collected metric data.
func TestNewMetrics_RecordsDuration(t *testing.T) {
	t.Parallel()
	mp, reader := newMeterProvider(t)

	r := chi.NewRouter()
	r.Use(middleware.NewMetrics(mp.Meter("test")))
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "http.server.request.duration" {
				found = true
			}
		}
	}
	if !found {
		t.Error("http.server.request.duration metric not found in collected data")
	}
}

// TestNewMetrics_CapturesStatusCode verifies that the response code written by
// the handler propagates through the wrapper to the actual response.
func TestNewMetrics_CapturesStatusCode(t *testing.T) {
	t.Parallel()
	mp, _ := newMeterProvider(t)

	r := chi.NewRouter()
	r.Use(middleware.NewMetrics(mp.Meter("test")))
	r.Get("/teapot", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/teapot", nil))
	if rr.Code != http.StatusTeapot {
		t.Errorf("response code: got %d; want %d", rr.Code, http.StatusTeapot)
	}
}

// TestNewMetrics_UsesChiRoutePattern verifies that requests with distinct IDs
// produce a single metric data point keyed to the route template, not the raw
// URL — keeping label cardinality bounded.
func TestNewMetrics_UsesChiRoutePattern(t *testing.T) {
	t.Parallel()
	mp, reader := newMeterProvider(t)

	r := chi.NewRouter()
	r.Use(middleware.NewMetrics(mp.Meter("test")))
	r.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, id := range []string{"1", "9999"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/"+id, nil))
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			histo, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatal("expected Histogram[float64] data")
			}
			for _, dp := range histo.DataPoints {
				for _, kv := range dp.Attributes.ToSlice() {
					if string(kv.Key) == "http.route" {
						got := kv.Value.AsString()
						if got == "/items/1" || got == "/items/9999" {
							t.Errorf("http.route should be template, got raw path %q", got)
						}
					}
				}
			}
		}
	}
}

// TestNewMetrics_FallsBackToRawPath verifies that when there is no chi context
// (e.g. the middleware is used outside a chi router) it falls back to the raw
// URL path without panicking.
func TestNewMetrics_FallsBackToRawPath(t *testing.T) {
	t.Parallel()
	mp, _ := newMeterProvider(t)

	h := middleware.NewMetrics(mp.Meter("test"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	// Plain ServeHTTP call — no chi.RouteContext in the request context.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/no-chi-context", nil))
}

// TestMetricsResponseWriter_Flush_Forwards verifies that Flush() is forwarded
// to the underlying writer when it implements http.Flusher.
func TestMetricsResponseWriter_Flush_Forwards(t *testing.T) {
	t.Parallel()
	mp, _ := newMeterProvider(t)

	spy := &spyFlusher{ResponseRecorder: httptest.NewRecorder()}
	h := middleware.NewMetrics(mp.Meter("test"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.(http.Flusher).Flush()
		}),
	)
	h.ServeHTTP(spy, httptest.NewRequest(http.MethodGet, "/", nil))

	if !spy.flushed {
		t.Error("expected Flush() to be forwarded to the underlying writer")
	}
}

// TestMetricsResponseWriter_Flush_NoOp verifies that calling Flush() does not
// panic when the underlying writer does not implement http.Flusher.
func TestMetricsResponseWriter_Flush_NoOp(t *testing.T) {
	t.Parallel()
	mp, _ := newMeterProvider(t)

	// opaqueWriter embeds http.ResponseWriter but does not implement http.Flusher,
	// so the type assertion in metricsResponseWriter.Flush() must return ok=false.
	base := &opaqueWriter{ResponseWriter: httptest.NewRecorder()}
	h := middleware.NewMetrics(mp.Meter("test"))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.(http.Flusher).Flush() // must not panic
		}),
	)
	h.ServeHTTP(base, httptest.NewRequest(http.MethodGet, "/", nil))
}
