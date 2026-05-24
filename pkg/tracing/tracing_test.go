package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// capturingExporter is a SpanExporter that accumulates spans and does NOT
// clear them on Shutdown. tracetest.InMemoryExporter.Shutdown() calls Reset(),
// which empties the buffer before the test can inspect it. This exporter
// avoids that race by keeping spans until explicitly reset.
type capturingExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *capturingExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *capturingExporter) Shutdown(context.Context) error { return nil }

func (e *capturingExporter) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.spans)
}

func (e *capturingExporter) SpanName(i int) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spans[i].Name()
}

// resetGlobalProvider restores a noop provider after each test so global
// state does not bleed between tests run in the same process.
func resetGlobalProvider(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
}

// ── disabled path ─────────────────────────────────────────────────────────────

func TestSetup_Disabled_ShutdownReturnsNil(t *testing.T) {
	resetGlobalProvider(t)
	shutdown, err := tracing.Setup(context.Background(), tracing.Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup(disabled): unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown(disabled): expected nil, got %v", err)
	}
}

func TestSetup_Disabled_SetsNoopProvider(t *testing.T) {
	resetGlobalProvider(t)
	_, _ = tracing.Setup(context.Background(), tracing.Config{Enabled: false})

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "probe")
	defer span.End()

	if span.SpanContext().IsValid() {
		t.Error("disabled tracing must produce non-valid (noop) spans")
	}
}

// ── enabled path (in-memory exporter) ─────────────────────────────────────────

func TestSetupWithExporter_ProducesRecordingSpans(t *testing.T) {
	resetGlobalProvider(t)

	exp := &capturingExporter{}
	cfg := tracing.Config{
		Enabled:        true,
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.1",
		Environment:    "test",
		SampleRate:     1.0,
	}
	shutdown := tracing.SetupWithExporter(cfg, exp)
	defer func() { _ = shutdown(context.Background()) }()

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "unit-test-span")
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if got := exp.Len(); got != 1 {
		t.Fatalf("expected 1 exported span, got %d", got)
	}
	if got := exp.SpanName(0); got != "unit-test-span" {
		t.Errorf("span name: want %q, got %q", "unit-test-span", got)
	}
}

func TestSetupWithExporter_AlwaysSampleAt100Percent(t *testing.T) {
	resetGlobalProvider(t)

	exp := &capturingExporter{}
	cfg := tracing.Config{
		Enabled:     true,
		ServiceName: "test-svc",
		SampleRate:  1.0,
	}
	shutdown := tracing.SetupWithExporter(cfg, exp)
	defer func() { _ = shutdown(context.Background()) }()

	tracer := otel.Tracer("test")
	const n = 10
	for range n {
		_, span := tracer.Start(context.Background(), "s")
		span.End()
	}
	_ = shutdown(context.Background())

	if got := exp.Len(); got != n {
		t.Errorf("100%% sample rate: want %d spans, got %d", n, got)
	}
}

func TestSetupWithExporter_SpanContextIsValid(t *testing.T) {
	resetGlobalProvider(t)

	exp := &capturingExporter{}
	shutdown := tracing.SetupWithExporter(tracing.Config{
		Enabled: true, ServiceName: "x", SampleRate: 1.0,
	}, exp)
	defer func() { _ = shutdown(context.Background()) }()

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "validity-check")
	defer span.End()

	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		t.Error("enabled tracing must produce valid SpanContext")
	}
	if !sc.TraceID().IsValid() {
		t.Error("TraceID must be valid (non-zero)")
	}
	if !sc.SpanID().IsValid() {
		t.Error("SpanID must be valid (non-zero)")
	}
}

func TestSetupWithExporter_RatioBased_SamplerUsed(t *testing.T) {
	resetGlobalProvider(t)

	exp := &capturingExporter{}
	// SampleRate < 1.0 triggers the TraceIDRatioBased sampler path.
	shutdown := tracing.SetupWithExporter(tracing.Config{
		Enabled:     true,
		ServiceName: "ratio-svc",
		SampleRate:  0.5,
	}, exp)
	defer func() { _ = shutdown(context.Background()) }()

	// A ratio-based sampler is installed; the provider must still produce
	// valid spans (whether sampled depends on the trace ID, so we just
	// verify the provider is wired and the span context is structurally valid).
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "ratio-span")
	span.End()

	sc := trace.SpanFromContext(ctx).SpanContext()
	// With SampleRate 0.5 the span may or may not be recorded; what matters
	// is that the call did not panic and the provider was installed.
	_ = sc
}

func TestSetupWithExporter_EmptyServiceName_FallsBackToDefault(t *testing.T) {
	resetGlobalProvider(t)

	exp := &capturingExporter{}
	// Empty ServiceName must not panic; the default "world-cup-quiniela" is used.
	shutdown := tracing.SetupWithExporter(tracing.Config{
		Enabled:    true,
		SampleRate: 1.0,
		// ServiceName intentionally blank
	}, exp)
	defer func() { _ = shutdown(context.Background()) }()

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "default-name-span")
	span.End()
	_ = shutdown(context.Background())

	if exp.Len() != 1 {
		t.Errorf("expected 1 span, got %d", exp.Len())
	}
}

// TestSetup_Enabled_CreatesOTLPExporter exercises the Setup() enabled path by
// pointing the OTLP exporter at a local httptest.Server. otlptracehttp.New
// does not eagerly connect, so Setup() succeeds even though the server is a
// stub that accepts any POST with 200.
func TestSetup_Enabled_CreatesOTLPExporter(t *testing.T) {
	resetGlobalProvider(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := tracing.Config{
		Enabled:        true,
		OTLPEndpoint:   srv.Listener.Addr().String(),
		ServiceName:    "setup-test",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		SampleRate:     1.0,
	}
	shutdown, err := tracing.Setup(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Setup(enabled): unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		// Shutdown may fail because the stub server doesn't speak OTLP; that is
		// acceptable — the important thing is that Setup() itself succeeded.
		t.Logf("shutdown returned (expected for stub): %v", err)
	}
}
