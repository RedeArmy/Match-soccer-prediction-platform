package service

import (
	"context"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap"
)

// ── RegisterGroupPrizeMetrics ─────────────────────────────────────────────────

func TestRegisterGroupPrizeMetrics_Succeeds(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := RegisterGroupPrizeMetrics(meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil GroupPrizeMetrics")
	}
	if m.DistributionFailuresTotal == nil {
		t.Error("DistributionFailuresTotal must not be nil after registration")
	}
}

// ── RecordDistributionFailure ─────────────────────────────────────────────────

func TestRecordDistributionFailure_NilReceiver_IsNoOp(t *testing.T) {
	var m *GroupPrizeMetrics
	m.RecordDistributionFailure(context.Background()) // must not panic
}

func TestRecordDistributionFailure_NilCounter_IsNoOp(t *testing.T) {
	m := &GroupPrizeMetrics{}                         // zero value: counter is nil
	m.RecordDistributionFailure(context.Background()) // must not panic
}

func TestRecordDistributionFailure_WithCounter_DoesNotPanic(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := RegisterGroupPrizeMetrics(meter)
	if err != nil {
		t.Fatalf("RegisterGroupPrizeMetrics: %v", err)
	}
	m.RecordDistributionFailure(context.Background()) // noop counter; must not panic
}

// ── SetPrizeMetrics wiring ────────────────────────────────────────────────────

func TestAdminGroupService_SetPrizeMetrics_NilSafe(t *testing.T) {
	svc := NewAdminGroupService(
		&stubQuinielaRepo{}, &stubMemberRepo{}, &noopSnapshotter{},
		&noopRanker{}, &noopAuditLogger{}, zap.NewNop(),
	)
	svc.(*adminGroupService).SetPrizeMetrics(nil) // nil is documented as safe
}

func TestAdminGroupService_SetPrizeMetrics_WiresMetrics(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, _ := RegisterGroupPrizeMetrics(meter)

	svc := NewAdminGroupService(
		&stubQuinielaRepo{}, &stubMemberRepo{}, &noopSnapshotter{},
		&noopRanker{}, &noopAuditLogger{}, zap.NewNop(),
	)
	svc.(*adminGroupService).SetPrizeMetrics(m)
	if svc.(*adminGroupService).prizeMetrics != m {
		t.Error("SetPrizeMetrics: prizeMetrics field not set correctly")
	}
}
