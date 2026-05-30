package service

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
)

// errOnNthCounterMeter embeds the noop meter and overrides Int64Counter to
// succeed for the first n-1 calls and fail on call n. This lets tests cover
// each error-return branch in newScoringMetrics independently.
type errOnNthCounterMeter struct {
	metricnoop.Meter
	failAt int // 1-based: fail on this call number
	called int
}

func (m *errOnNthCounterMeter) Int64Counter(name string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	m.called++
	if m.called == m.failAt {
		return nil, errors.New("intentional test error on counter " + name)
	}
	return metricnoop.Int64Counter{}, nil
}

func TestNewScoringMetrics_WithNoopMeter_Succeeds(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := newScoringMetrics(meter)
	if err != nil {
		t.Fatalf("newScoringMetrics: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil scoringMetrics")
	}
	if m.predictionsScored == nil {
		t.Error("predictionsScored counter should not be nil")
	}
	if m.batchChunks == nil {
		t.Error("batchChunks counter should not be nil")
	}
}

func TestScoringMetrics_Observe_WithRealCounters_DoesNotPanic(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := newScoringMetrics(meter)
	if err != nil {
		t.Fatalf("newScoringMetrics: %v", err)
	}
	// Must not panic for any combination of predictions/chunks.
	m.observe(context.Background(), 505, 2)
	m.observe(context.Background(), 0, 0)
}

func TestScoringMetrics_Observe_NilReceiver_IsNoop(t *testing.T) {
	var m *scoringMetrics
	// Must not panic.
	m.observe(context.Background(), 10, 1)
}

// TestNewScoringMetrics_CounterError_ReturnsError covers both error-return
// branches in newScoringMetrics: the first when predictions_scored fails, the
// second when batch_chunks fails. Defensive code — never triggered in prod.
func TestNewScoringMetrics_FirstCounterError_ReturnsError(t *testing.T) {
	m, err := newScoringMetrics(&errOnNthCounterMeter{failAt: 1})
	if err == nil {
		t.Fatal("expected error from failing meter (first counter), got nil")
	}
	if m != nil {
		t.Error("expected nil scoringMetrics on error")
	}
}

func TestNewScoringMetrics_SecondCounterError_ReturnsError(t *testing.T) {
	m, err := newScoringMetrics(&errOnNthCounterMeter{failAt: 2})
	if err == nil {
		t.Fatal("expected error from failing meter (second counter), got nil")
	}
	if m != nil {
		t.Error("expected nil scoringMetrics on error")
	}
}
