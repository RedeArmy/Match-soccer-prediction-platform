package breaker_test

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

func TestRegisterGauge_ReportsCorrectState(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("test")

	b := breaker.New("db", 3, 0)

	if err := breaker.RegisterGauge(meter, b); err != nil {
		t.Fatalf("RegisterGauge: %v", err)
	}

	collect := func() map[string]float64 {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &rm); err != nil {
			t.Fatalf("collect metrics: %v", err)
		}
		vals := map[string]float64{}
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if m.Name != "circuit_breaker.state" {
					continue
				}
				if gd, ok := m.Data.(metricdata.Gauge[float64]); ok {
					for _, dp := range gd.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "backend" {
								vals[attr.Value.AsString()] = dp.Value
							}
						}
					}
				}
			}
		}
		return vals
	}

	// Closed state (0).
	vals := collect()
	if got := vals["db"]; got != float64(breaker.StateClosed) {
		t.Errorf("closed state: got %v; want %v", got, float64(breaker.StateClosed))
	}

	// Trip the breaker to Open (1).
	for i := 0; i < 3; i++ {
		_ = b.Call(func() error { return errFake })
	}
	vals = collect()
	if got := vals["db"]; got != float64(breaker.StateOpen) {
		t.Errorf("open state: got %v; want %v", got, float64(breaker.StateOpen))
	}
}

func TestRegisterGauge_MultipleBreakers(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("test")

	a := breaker.New("a", 1, 0)
	b2 := breaker.New("b", 1, 0)

	if err := breaker.RegisterGauge(meter, a, b2); err != nil {
		t.Fatalf("RegisterGauge: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	seen := 0
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "circuit_breaker.state" {
				if gd, ok := m.Data.(metricdata.Gauge[float64]); ok {
					seen = len(gd.DataPoints)
				}
			}
		}
	}
	if seen != 2 {
		t.Errorf("data points: got %d; want 2", seen)
	}
}

type fakeError string

func (e fakeError) Error() string { return string(e) }

const errFake fakeError = "fake"
