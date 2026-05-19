package tracing

import (
	"context"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SetupWithExporter is exposed for testing only. It wires cfg with the
// provided exporter instead of creating an OTLP HTTP connection.
func SetupWithExporter(cfg Config, exporter sdktrace.SpanExporter) func(context.Context) error {
	return setupWithExporter(cfg, exporter)
}
