package service

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// scoringMetrics holds the OTel counters emitted by scoringService after each
// successful ScoreMatch run. Construct via newScoringMetrics; nil is safe and
// means no metrics are recorded.
type scoringMetrics struct {
	predictionsScored metric.Int64Counter
	batchChunks       metric.Int64Counter
}

func newScoringMetrics(meter metric.Meter) (*scoringMetrics, error) {
	m := &scoringMetrics{}
	var err error

	m.predictionsScored, err = meter.Int64Counter(
		"scoring.predictions_scored",
		metric.WithDescription("Total number of predictions updated across all scoring runs."),
	)
	if err != nil {
		return nil, fmt.Errorf("scoring metrics: predictions_scored: %w", err)
	}

	m.batchChunks, err = meter.Int64Counter(
		"scoring.batch_chunks",
		metric.WithDescription(
			"Total number of 500-row UPDATE chunks executed. "+
				"chunk_count = ceil(prediction_count / 500) per scoring run.",
		),
	)
	if err != nil {
		return nil, fmt.Errorf("scoring metrics: batch_chunks: %w", err)
	}

	return m, nil
}

// observe increments the counters for one completed ScoreMatch run.
// Safe to call on a nil receiver — no-op when metrics are not wired.
func (m *scoringMetrics) observe(ctx context.Context, predictions, chunks int64) {
	if m == nil {
		return
	}
	if m.predictionsScored != nil {
		m.predictionsScored.Add(ctx, predictions)
	}
	if m.batchChunks != nil {
		m.batchChunks.Add(ctx, chunks)
	}
}
