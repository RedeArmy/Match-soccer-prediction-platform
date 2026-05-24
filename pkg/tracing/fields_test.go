package tracing_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

func TestLogFields_NoSpan_ReturnsNil(t *testing.T) {
	t.Parallel()
	fields := tracing.LogFields(context.Background())
	if fields != nil {
		t.Errorf("expected nil for context without span, got %v", fields)
	}
}

func TestLogFields_ValidSpan_ReturnsTwoFields(t *testing.T) {
	t.Parallel()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	fields := tracing.LogFields(ctx)
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields (trace_id, span_id), got %d", len(fields))
	}

	// Verify field keys using the string representation of zap.Field.
	// zap.Field has a Key string attribute.
	keys := map[string]bool{}
	for _, f := range fields {
		keys[f.Key] = true
	}
	if !keys["trace_id"] {
		t.Error("expected field with key 'trace_id'")
	}
	if !keys["span_id"] {
		t.Error("expected field with key 'span_id'")
	}
}

func TestLogFields_NoopSpan_ReturnsNil(t *testing.T) {
	t.Parallel()
	// noop provider produces non-recording spans with invalid SpanContext
	tracer := otel.Tracer("noop")
	ctx, span := tracer.Start(context.Background(), "noop-span")
	defer span.End()

	fields := tracing.LogFields(ctx)
	if fields != nil {
		t.Errorf("expected nil for noop span, got %v", fields)
	}
}
