package tracing

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// LogFields returns zap fields for the trace_id and span_id present in ctx.
// Returns nil when ctx carries no valid span (tracing disabled or no active span).
// Safe to splat into a []zap.Field with append or pass directly to log.With.
func LogFields(ctx context.Context) []zap.Field {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return nil
	}
	return []zap.Field{
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	}
}
