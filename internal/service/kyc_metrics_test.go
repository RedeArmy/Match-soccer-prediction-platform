package service

import (
	"context"
	"errors"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
)

func newTestMetrics(t *testing.T) *KYCMetrics {
	t.Helper()
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := RegisterKYCMetrics(meter, nil, nil)
	if err != nil {
		t.Fatalf("RegisterKYCMetrics: %v", err)
	}
	return m
}

// ── RegisterKYCMetrics ────────────────────────────────────────────────────────

func TestRegisterKYCMetrics_WithNilReaders_Succeeds(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := RegisterKYCMetrics(meter, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil KYCMetrics")
	}
}

func TestRegisterKYCMetrics_WithQueueReader_Succeeds(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	reader := &fixedQueueReader{n: 5}
	m, err := RegisterKYCMetrics(meter, reader, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil KYCMetrics")
	}
}

func TestRegisterKYCMetrics_WithFrozenReader_Succeeds(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	reader := &fixedFrozenReader{cents: 100_000}
	m, err := RegisterKYCMetrics(meter, nil, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil KYCMetrics")
	}
}

func TestRegisterKYCMetrics_WithBothReaders_Succeeds(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := RegisterKYCMetrics(meter, &fixedQueueReader{}, &fixedFrozenReader{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil KYCMetrics")
	}
}

// ── nil receiver guards ───────────────────────────────────────────────────────

func TestKYCMetrics_RecordSubmission_NilReceiver_NoPanic(t *testing.T) {
	var m *KYCMetrics
	m.RecordSubmission(context.Background(), "success")
}

func TestKYCMetrics_RecordReviewDuration_NilReceiver_NoPanic(t *testing.T) {
	var m *KYCMetrics
	m.RecordReviewDuration(context.Background(), 120.0, "approved")
}

func TestKYCMetrics_RecordFraudFlag_NilReceiver_NoPanic(t *testing.T) {
	var m *KYCMetrics
	m.RecordFraudFlag(context.Background(), "pep")
}

func TestKYCMetrics_RecordGateBlock_NilReceiver_NoPanic(t *testing.T) {
	var m *KYCMetrics
	m.RecordGateBlock(context.Background(), "deposit", "cap_exceeded")
}

// ── happy-path: methods do not panic with a real (noop) meter ─────────────────

func TestKYCMetrics_RecordSubmission_DoesNotPanic(t *testing.T) {
	m := newTestMetrics(t)
	m.RecordSubmission(context.Background(), "success")
	m.RecordSubmission(context.Background(), "conflict")
	m.RecordSubmission(context.Background(), "validation_error")
}

func TestKYCMetrics_RecordReviewDuration_DoesNotPanic(t *testing.T) {
	m := newTestMetrics(t)
	m.RecordReviewDuration(context.Background(), 300.0, "approved")
	m.RecordReviewDuration(context.Background(), 86400.0, "rejected")
}

func TestKYCMetrics_RecordFraudFlag_DoesNotPanic(t *testing.T) {
	m := newTestMetrics(t)
	m.RecordFraudFlag(context.Background(), "pep")
	m.RecordFraudFlag(context.Background(), "sanctions")
}

func TestKYCMetrics_RecordGateBlock_DoesNotPanic(t *testing.T) {
	m := newTestMetrics(t)
	m.RecordGateBlock(context.Background(), "deposit", "cap_exceeded")
	m.RecordGateBlock(context.Background(), "withdrawal", "tier_insufficient")
	m.RecordGateBlock(context.Background(), "deposit", "velocity_exceeded")
}

// ── observer stubs ────────────────────────────────────────────────────────────

type fixedQueueReader struct {
	n   int64
	err error
}

func (r *fixedQueueReader) CountReviewQueue(_ context.Context) (int64, error) {
	return r.n, r.err
}

type fixedFrozenReader struct {
	cents int64
	err   error
}

func (r *fixedFrozenReader) SumFrozenAmountCents(_ context.Context) (int64, error) {
	return r.cents, r.err
}

// ── reader error paths: callbacks must not propagate errors to the SDK ────────

func TestRegisterKYCMetrics_QueueReaderError_DoesNotBlockRegistration(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	reader := &fixedQueueReader{err: errors.New("db down")}
	if _, err := RegisterKYCMetrics(meter, reader, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterKYCMetrics_FrozenReaderError_DoesNotBlockRegistration(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	reader := &fixedFrozenReader{err: errors.New("db down")}
	if _, err := RegisterKYCMetrics(meter, nil, reader); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCMetrics_RecordAMLHit_NilReceiver_NoPanic(t *testing.T) {
	var m *KYCMetrics
	m.RecordAMLHit(context.Background())
}

func TestKYCMetrics_RecordAMLHit_DoesNotPanic(t *testing.T) {
	m := newTestMetrics(t)
	m.RecordAMLHit(context.Background())
}
