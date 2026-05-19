// Package tracing initialises the global OpenTelemetry TracerProvider and
// TextMapPropagator for the application. Wire Setup once at process startup
// (composition root) and defer the returned shutdown function so pending spans
// are flushed before the process exits.
//
// When Enabled is false a no-op TracerProvider is installed: all calls to
// trace.SpanFromContext and otel.Tracer return non-recording spans with zero
// allocation overhead. This makes disabling tracing in development a
// configuration change, not a code change — handlers never branch on whether
// tracing is active.
//
// When Enabled is true an OTLP HTTP exporter ships spans to the configured
// endpoint (Grafana Tempo, Jaeger, DataDog Agent, any OTel Collector). The
// W3C TraceContext propagator is installed globally so that upstream
// traceparent / tracestate headers are honoured, enabling end-to-end traces
// that span the frontend, load balancer, and API server.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config holds the tracing configuration resolved from environment variables.
type Config struct {
	// Enabled controls whether spans are collected and exported.
	// Set WCQ_TRACING_ENABLED=true in production. Defaults to false.
	Enabled bool
	// OTLPEndpoint is the base URL of the OTLP HTTP receiver, e.g.
	// "http://tempo:4318". Required when Enabled is true.
	OTLPEndpoint string
	// ServiceName is stamped on every span. Defaults to "world-cup-quiniela".
	ServiceName string
	// ServiceVersion is stamped on every span. Defaults to "1.0.0".
	ServiceVersion string
	// Environment is stamped on every span (e.g. "production", "staging").
	Environment string
	// SampleRate controls the fraction of traces to record (0.0–1.0).
	// 1.0 records every trace; 0.1 records 10%. Defaults to 1.0.
	SampleRate float64
}

// Setup initialises the global TracerProvider and installs the W3C TraceContext
// propagator. The returned shutdown function must be deferred by the caller:
// it flushes the export queue and releases the exporter connection.
//
// When cfg.Enabled is false, a no-op provider is installed and the returned
// shutdown function is a no-op itself — no network connections are made.
//
// Setup is not safe to call concurrently. Call it exactly once, before the
// HTTP server starts accepting traffic.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: create OTLP exporter: %w", err)
	}

	return setupWithExporter(cfg, exporter), nil
}

// setupWithExporter wires a TracerProvider backed by the given exporter.
// Extracted from Setup so tests can inject an in-memory exporter without a
// real OTLP endpoint.
func setupWithExporter(cfg Config, exporter sdktrace.SpanExporter) func(context.Context) error {
	svcName := cfg.ServiceName
	if svcName == "" {
		svcName = "world-cup-quiniela"
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			attribute.String("service.name", svcName),
			attribute.String("service.version", cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)

	sampler := sdktrace.TraceIDRatioBased(cfg.SampleRate)
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown
}
