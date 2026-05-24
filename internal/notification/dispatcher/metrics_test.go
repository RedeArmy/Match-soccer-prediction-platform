package dispatcher_test

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
)

func newTestMeter(t *testing.T) sdkmetric.Reader {
	t.Helper()
	return sdkmetric.NewManualReader()
}

func TestUserDispatcher_RegisterMetrics_Succeeds(t *testing.T) {
	reader := newTestMeter(t)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		Log: zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

func TestAdminDispatcher_RegisterMetrics_Succeeds(t *testing.T) {
	reader := newTestMeter(t)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	d := dispatcher.NewAdminDispatcher(dispatcher.Config{
		Log: zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

func TestUserDispatcher_RegisterMetrics_InstrumentsPresent(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		Log: zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	// Verify instruments can be collected without error.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
}
