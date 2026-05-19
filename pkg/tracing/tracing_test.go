package tracing_test

import (
	"context"
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
