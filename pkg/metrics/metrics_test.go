package metrics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"

	"go.opentelemetry.io/otel"

	"github.com/rede/world-cup-quiniela/pkg/metrics"
)

// resetGlobalProvider restores a noop MeterProvider after each test so global
// state does not bleed between tests run in the same process.
func resetGlobalMeterProvider(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
	})
}

// ── disabled path ──────────────────────────────────────────────────────────────

func TestSetup_Disabled_HandlerIsNil(t *testing.T) {
	resetGlobalMeterProvider(t)
	_, handler, shutdown, err := metrics.Setup(metrics.Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup(disabled): unexpected error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	if handler != nil {
		t.Error("disabled metrics must return nil handler")
	}
}

func TestSetup_Disabled_ShutdownReturnsNil(t *testing.T) {
	resetGlobalMeterProvider(t)
	_, _, shutdown, err := metrics.Setup(metrics.Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup(disabled): unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown(disabled): expected nil, got %v", err)
	}
}

func TestSetup_Disabled_MeterIsUsable(t *testing.T) {
	resetGlobalMeterProvider(t)
	meter, _, shutdown, err := metrics.Setup(metrics.Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup(disabled): unexpected error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	// Creating instruments on the noop meter must not panic.
	_, createErr := meter.Int64Counter("test.counter")
	if createErr != nil {
		t.Errorf("noop meter: Int64Counter returned error: %v", createErr)
	}
}

// ── enabled path ──────────────────────────────────────────────────────────────

func TestSetup_Enabled_HandlerIsNotNil(t *testing.T) {
	resetGlobalMeterProvider(t)
	_, handler, shutdown, err := metrics.Setup(metrics.Config{Enabled: true, Namespace: "test"})
	if err != nil {
		t.Fatalf("Setup(enabled): unexpected error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
	if handler == nil {
		t.Error("enabled metrics must return a non-nil http.Handler")
	}
}

func TestSetup_Enabled_HandlerResponds200(t *testing.T) {
	resetGlobalMeterProvider(t)
	_, handler, shutdown, err := metrics.Setup(metrics.Config{Enabled: true, Namespace: "test2"})
	if err != nil {
		t.Fatalf("Setup(enabled): unexpected error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/metrics handler: want 200, got %d", rr.Code)
	}
}

func TestSetup_Enabled_MeterInstrumentUsable(t *testing.T) {
	resetGlobalMeterProvider(t)
	meter, _, shutdown, err := metrics.Setup(metrics.Config{Enabled: true, Namespace: "test3"})
	if err != nil {
		t.Fatalf("Setup(enabled): unexpected error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	counter, err := meter.Int64Counter("test.requests")
	if err != nil {
		t.Fatalf("Int64Counter: unexpected error: %v", err)
	}
	counter.Add(context.Background(), 1) // must not panic
}

func TestSetup_Enabled_ShutdownReturnsNil(t *testing.T) {
	resetGlobalMeterProvider(t)
	_, _, shutdown, err := metrics.Setup(metrics.Config{Enabled: true, Namespace: "test4"})
	if err != nil {
		t.Fatalf("Setup(enabled): %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown(enabled): expected nil, got %v", err)
	}
}

func TestSetup_Enabled_EmptyNamespace_UsesDefault(t *testing.T) {
	resetGlobalMeterProvider(t)
	// Empty Namespace must not panic; defaults to "wcq".
	meter, _, shutdown, err := metrics.Setup(metrics.Config{Enabled: true})
	if err != nil {
		t.Fatalf("Setup(empty namespace): unexpected error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	if meter == nil {
		t.Error("meter must not be nil when enabled")
	}
}
