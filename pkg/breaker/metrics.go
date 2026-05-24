package breaker

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RegisterGauge registers an observable gauge that reports the current state
// of each supplied breaker as a numeric value (0 = closed, 1 = open,
// 2 = half-open) with a "backend" attribute set to the breaker's name.
//
// Call once per meter at startup. Safe to call multiple times with the same
// meter (each call registers a new callback, which is harmless but wasteful).
func RegisterGauge(meter metric.Meter, breakers ...*Breaker) error {
	_, err := meter.Float64ObservableGauge(
		"circuit_breaker.state",
		metric.WithDescription("Circuit breaker state: 0=closed, 1=open, 2=half-open."),
		metric.WithFloat64Callback(func(_ context.Context, obs metric.Float64Observer) error {
			for _, b := range breakers {
				obs.Observe(float64(b.CurrentState()),
					metric.WithAttributes(attribute.String("backend", b.Name())))
			}
			return nil
		}),
	)
	return err
}
