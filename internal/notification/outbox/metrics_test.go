package outbox_test

import (
	"context"
	"errors"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// checkFloat64GaugeDataPoints validates every data point in m against want.
func checkFloat64GaugeDataPoints(t *testing.T, m metricdata.Metric, name string, want float64) {
	t.Helper()
	gd, ok := m.Data.(metricdata.Gauge[float64])
	if !ok {
		return
	}
	for _, dp := range gd.DataPoints {
		if dp.Value != want {
			t.Errorf("%s: expected %v, got %v", name, want, dp.Value)
		}
	}
}

// assertFloat64GaugeAllEqual fails if any data point of the named gauge in rm
// differs from want. Extracted to keep individual test bodies below the
// gocognit threshold.
func assertFloat64GaugeAllEqual(t *testing.T, rm metricdata.ResourceMetrics, name string, want float64) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			checkFloat64GaugeDataPoints(t, m, name, want)
		}
	}
}

// findInt64GaugeValue returns the first data-point value and true for the
// named int64 Gauge in rm, or 0 and false when the metric is absent.
func findInt64GaugeValue(rm metricdata.ResourceMetrics, name string) (int64, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			gd, ok := m.Data.(metricdata.Gauge[int64])
			if !ok || len(gd.DataPoints) == 0 {
				return 0, false
			}
			return gd.DataPoints[0].Value, true
		}
	}
	return 0, false
}

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubDLQDepthReader struct {
	n   int64
	err error
}

func (s *stubDLQDepthReader) CountUnresolved(_ context.Context) (int64, error) {
	return s.n, s.err
}

func TestCountPending_ZeroWhenEmpty(t *testing.T) {
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "TRUNCATE domain_outbox RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	repo := outbox.NewPostgresRepository(testPool)
	n, err := repo.CountPending(ctx)
	if err != nil {
		t.Fatalf("CountPending: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 pending; got %d", n)
	}
}

func TestCountPending_CountsDueRows(t *testing.T) {
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "TRUNCATE domain_outbox RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Insert 2 pending rows with scheduled_at = NOW() (due immediately).
	insert := `
INSERT INTO domain_outbox (event_type, aggregate_id, aggregate_type, payload, status, max_attempts, scheduled_at)
VALUES ($1, '1', 'test', '{}', 'pending', 3, NOW())
`
	if _, err := testPool.Exec(ctx, insert, "test.event.a"); err != nil {
		t.Fatalf("insert row 1: %v", err)
	}
	if _, err := testPool.Exec(ctx, insert, "test.event.b"); err != nil {
		t.Fatalf("insert row 2: %v", err)
	}
	// Insert 1 future row (not yet due).
	future := `
INSERT INTO domain_outbox (event_type, aggregate_id, aggregate_type, payload, status, max_attempts, scheduled_at)
VALUES ('test.event.c', '1', 'test', '{}', 'pending', 3, NOW() + interval '1 hour')
`
	if _, err := testPool.Exec(ctx, future); err != nil {
		t.Fatalf("insert future row: %v", err)
	}

	repo := outbox.NewPostgresRepository(testPool)
	n, err := repo.CountPending(ctx)
	if err != nil {
		t.Fatalf("CountPending: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 pending due rows; got %d", n)
	}
}

// ── RegisterDLQDepthGauge ─────────────────────────────────────────────────────

func TestRegisterDLQDepthGauge_Succeeds(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	if err := outbox.RegisterDLQDepthGauge(mp.Meter("test"), &stubDLQDepthReader{n: 3}); err != nil {
		t.Fatalf("RegisterDLQDepthGauge: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	val, ok := findInt64GaugeValue(rm, "outbox.dlq_depth")
	if !ok {
		t.Error("expected 'outbox.dlq_depth' gauge to be present after collection")
		return
	}
	if val != 3 {
		t.Errorf("expected dlq_depth=3, got %d", val)
	}
}

func TestRegisterDLQDepthGauge_ReaderError_DoesNotFail(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	stub := &stubDLQDepthReader{err: errors.New("db down")}
	if err := outbox.RegisterDLQDepthGauge(mp.Meter("test"), stub); err != nil {
		t.Fatalf("RegisterDLQDepthGauge: %v", err)
	}
	// Collection must not panic or return an error even when the DB query fails.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect should not fail when callback swallows DB error: %v", err)
	}
}

// ── RegisterOldestPendingAgeGauge ────────────────────────────────────────────

func TestRegisterOldestPendingAgeGauge_Succeeds(t *testing.T) {
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "TRUNCATE domain_outbox RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	repo := outbox.NewPostgresRepository(testPool)
	if err := outbox.RegisterOldestPendingAgeGauge(mp.Meter("test"), repo); err != nil {
		t.Fatalf("RegisterOldestPendingAgeGauge: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "outbox.oldest_pending_age_seconds" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected 'outbox.oldest_pending_age_seconds' gauge to be present after collection")
	}
}

func TestRegisterOldestPendingAgeGauge_EmptyQueue_ReturnsZero(t *testing.T) {
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "TRUNCATE domain_outbox RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	repo := outbox.NewPostgresRepository(testPool)
	if err := outbox.RegisterOldestPendingAgeGauge(mp.Meter("test"), repo); err != nil {
		t.Fatalf("RegisterOldestPendingAgeGauge: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertFloat64GaugeAllEqual(t, rm, "outbox.oldest_pending_age_seconds", 0)
}

// ── RegisterPendingGauge ──────────────────────────────────────────────────────

func TestRegisterPendingGauge_Succeeds(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	repo := outbox.NewPostgresRepository(testPool)
	if err := outbox.RegisterPendingGauge(mp.Meter("test"), repo); err != nil {
		t.Fatalf("RegisterPendingGauge: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "outbox.pending_events" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected 'outbox.pending_events' gauge to be present after collection")
	}
}
