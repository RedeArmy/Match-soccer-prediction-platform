// Package metrics initialises the global OpenTelemetry MeterProvider backed
// by a Prometheus exporter and exposes the /metrics scrape handler.
//
// Usage — composition root (cmd/api/setup.go):
//
//	meter, handler, shutdown, err := metrics.Setup(metrics.Config{
//	    Enabled:   cfg.Metrics.Enabled,
//	    Namespace: cfg.Metrics.Namespace,
//	})
//
// When Enabled is false a noop MeterProvider is installed and handler is nil
// — no Prometheus registry is created and no network port is opened.
package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"go.opentelemetry.io/otel"
)

// Config holds the Prometheus metrics configuration.
type Config struct {
	// Enabled controls whether the Prometheus exporter is active.
	// Default: false. Set WCQ_METRICS_ENABLED=true in production.
	Enabled bool
	// Namespace is the metric name prefix applied to all instruments.
	// Defaults to "wcq" when empty.
	Namespace string
}

// Setup initialises the global OTel MeterProvider and returns:
//   - meter: a Meter scoped to Namespace (defaults to "wcq")
//   - handler: the Prometheus HTTP handler for /metrics (nil when disabled)
//   - shutdown: flushes any in-flight collections; must be deferred by caller
//   - error: non-nil only if the Prometheus exporter fails to initialise
//
// When Enabled is false, a noop MeterProvider is installed so that all OTel
// Meter calls in the application compile and execute without side-effects.
func Setup(cfg Config) (metric.Meter, http.Handler, func(context.Context) error, error) {
	if !cfg.Enabled {
		noopMp := metricnoop.NewMeterProvider()
		otel.SetMeterProvider(noopMp)
		return noopMp.Meter(""), nil, func(context.Context) error { return nil }, nil
	}

	if cfg.Namespace == "" {
		cfg.Namespace = "wcq"
	}

	exp, err := promexporter.New()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("metrics: prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp))
	otel.SetMeterProvider(mp)

	shutdown := func(ctx context.Context) error {
		return mp.Shutdown(ctx)
	}
	return mp.Meter(cfg.Namespace), promhttp.Handler(), shutdown, nil
}
