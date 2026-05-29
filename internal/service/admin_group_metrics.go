package service

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// GroupPrizeMetrics holds the OTel instruments used to observe prize
// distribution operations. Construct once at startup via RegisterGroupPrizeMetrics
// and wire into adminGroupService via SetPrizeMetrics.
type GroupPrizeMetrics struct {
	// DistributionFailuresTotal counts prize distribution attempts that failed
	// after prizes_distributed_at was stamped, leaving the pool in a partially
	// credited state. Any non-zero value requires immediate investigation.
	DistributionFailuresTotal metric.Int64Counter
}

// RegisterGroupPrizeMetrics creates and registers the group prize OTel
// instruments on meter. Returns a ready-to-use GroupPrizeMetrics or an error
// if instrument creation fails (e.g. duplicate name on a shared meter).
func RegisterGroupPrizeMetrics(meter metric.Meter) (*GroupPrizeMetrics, error) {
	m := &GroupPrizeMetrics{}
	var err error

	m.DistributionFailuresTotal, err = meter.Int64Counter(
		"prize.distribution_failures_total",
		metric.WithDescription(
			"Number of prize distribution operations that failed after "+
				"prizes_distributed_at was stamped. Non-zero indicates a partial "+
				"credit state that requires manual reconciliation.",
		),
	)
	if err != nil {
		return nil, fmt.Errorf("group prize metrics: distribution_failures_total: %w", err)
	}

	return m, nil
}

// RecordDistributionFailure increments the distribution failures counter.
// Safe to call on a nil receiver — no-op when metrics are not wired.
func (m *GroupPrizeMetrics) RecordDistributionFailure(ctx context.Context) {
	if m == nil || m.DistributionFailuresTotal == nil {
		return
	}
	m.DistributionFailuresTotal.Add(ctx, 1)
}
