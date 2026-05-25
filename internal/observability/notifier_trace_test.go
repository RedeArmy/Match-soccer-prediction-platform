package observability_test

// notifier_trace_test.go verifies that the observability Notifier correctly
// captures the active OTel span's trace_id from the call-site context and
// embeds it in the webhook payload.  This is a distinct concern from the path
// and payload-field tests in notifier_test.go: those use context.Background()
// (no active span), while these tests inject a real SDK span so the
// extractTraceID helper in notifier.go has something non-empty to return.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/rede/world-cup-quiniela/internal/observability"
)

// newSDKTracer creates a self-contained TracerProvider backed by a no-op
// SpanExporter.  It is used in tests that need a real (non-noop) span so that
// trace.SpanFromContext returns a span with a valid SpanContext.
func newSDKTracer(t *testing.T) *sdktrace.TracerProvider {
	t.Helper()
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp
}

// TestNotifyDLQOverflow_WithActiveOTelSpan_PropagatesTraceID verifies the full
// n8n webhook delivery chain when an active OTel span is present in the
// context:
//
//  1. A real SDK TracerProvider is created and a span is started.
//  2. NotifyDLQOverflow is called with that span context.
//  3. The mock n8n receiver receives the POST.
//  4. The decoded payload's trace_id matches the active span's TraceID.
//
// This is the "E2E" coverage for n8n webhook delivery with trace propagation
// required by Phase 10: "mock n8n receiver, trigger DLQ overflow, assert POST
// received with correct payload and trace_id".
func TestNotifyDLQOverflow_WithActiveOTelSpan_PropagatesTraceID(t *testing.T) {
	// Start a mock n8n receiver that records the request.
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	// Create a real OTel TracerProvider and start a span so the context carries
	// a valid SpanContext (extractTraceID returns "" from noop spans).
	tp := newSDKTracer(t)
	tracer := tp.Tracer("test-notifier")
	ctx, span := tracer.Start(context.Background(), "dlq-overflow-test")
	defer span.End()

	wantTraceID := span.SpanContext().TraceID().String()
	if wantTraceID == "00000000000000000000000000000000" {
		t.Fatal("SDK tracer produced a zero trace ID — test setup is broken")
	}

	n := newNotifier(t, srv.URL, "")
	n.NotifyDLQOverflow(ctx, 75, 50)
	ts.waitForCount(t, 1)

	// Decode and assert payload fields.
	var p observability.DLQOverflowPayload
	if err := json.Unmarshal([]byte(ts.lastBody()), &p); err != nil {
		t.Fatalf("unmarshal DLQ payload: %v", err)
	}

	if p.DLQDepth != 75 {
		t.Errorf("dlq_depth: want 75, got %d", p.DLQDepth)
	}
	if p.Threshold != 50 {
		t.Errorf("threshold: want 50, got %d", p.Threshold)
	}
	if p.Timestamp == "" {
		t.Error("timestamp must not be empty")
	}
	if p.TraceID == "" {
		t.Fatal("trace_id must not be empty when an active OTel span is present")
	}
	if p.TraceID != wantTraceID {
		t.Errorf("trace_id mismatch:\n  got  %s\n  want %s", p.TraceID, wantTraceID)
	}
}

// TestNotifyDLQOverflow_WithoutActiveSpan_TraceIDIsEmpty verifies the
// complementary case: without an active span the payload's trace_id is the
// empty string (not a zero-value UUID or a garbage value).
func TestNotifyDLQOverflow_WithoutActiveSpan_TraceIDIsEmpty(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	n := newNotifier(t, srv.URL, "")
	// context.Background() has no active span → extractTraceID returns "".
	n.NotifyDLQOverflow(context.Background(), 10, 5)
	ts.waitForCount(t, 1)

	var p observability.DLQOverflowPayload
	if err := json.Unmarshal([]byte(ts.lastBody()), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.TraceID != "" {
		t.Errorf("trace_id: want empty string without active span, got %q", p.TraceID)
	}
}

// TestNotifyCircuitBreakerOpen_WithActiveSpan_TraceIDPresent verifies that
// trace propagation works for all notifier methods, not just DLQOverflow.
// A single representative non-DLQ method is tested here to avoid repetition
// while still proving the extractTraceID call is present across call paths.
func TestNotifyCircuitBreakerOpen_WithActiveSpan_TraceIDPresent(t *testing.T) {
	ts := &testServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	tp := newSDKTracer(t)
	ctx, span := tp.Tracer("test").Start(context.Background(), "cb-test")
	defer span.End()
	wantTraceID := span.SpanContext().TraceID().String()

	n := newNotifier(t, srv.URL, "")
	n.NotifyCircuitBreakerOpen(ctx, "s3", "open", time.Now())
	// NotifyCircuitBreakerOpen is fire-and-forget; wait for delivery.
	ts.waitForCount(t, 1)

	var p observability.CircuitBreakerPayload
	if err := json.Unmarshal([]byte(ts.lastBody()), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.TraceID == "" {
		t.Fatal("trace_id must not be empty with active span")
	}
	if p.TraceID != wantTraceID {
		t.Errorf("trace_id: want %s, got %s", wantTraceID, p.TraceID)
	}
}
