package outbox

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

// DLQDepthReader is the narrow read interface consumed by RegisterDLQDepthGauge.
// It is satisfied by *repository.postgresNotificationDLQRepository.
type DLQDepthReader interface {
	CountUnresolved(ctx context.Context) (int64, error)
}

// RegisterPendingGauge registers an observable gauge that reports the number
// of pending-and-due outbox rows.  The gauge is sampled on each Prometheus
// scrape (or OTel collection interval), so it reflects near-real-time backlog
// depth without adding per-dispatch overhead.
//
// Call once at worker startup after the repository is available.
func RegisterPendingGauge(meter metric.Meter, repo Repository) error {
	_, err := meter.Int64ObservableGauge(
		"outbox.pending_events",
		metric.WithDescription("Number of outbox rows in 'pending' status that are due for processing."),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			n, err := repo.CountPending(ctx)
			if err != nil {
				return nil // swallow; don't fail the collection cycle
			}
			obs.Observe(n)
			return nil
		}),
	)
	return err
}

// RegisterDLQDepthGauge registers an observable gauge that reports the number
// of unresolved entries in the notification DLQ.  A sustained non-zero value
// means failed dispatch attempts are not being replayed and notifications are
// being lost.
//
// Call once at worker startup after the DLQ repository is available.
func RegisterDLQDepthGauge(meter metric.Meter, repo DLQDepthReader) error {
	_, err := meter.Int64ObservableGauge(
		"outbox.dlq_depth",
		metric.WithDescription("Number of unresolved entries in the notification DLQ (notification_dlq WHERE resolved_at IS NULL)."),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			n, err := repo.CountUnresolved(ctx)
			if err != nil {
				return nil
			}
			obs.Observe(n)
			return nil
		}),
	)
	return err
}

// RegisterOldestPendingAgeGauge registers an observable gauge that reports the
// age in seconds of the oldest pending-due outbox row.  Zero when the queue is
// empty.  Sustained non-zero values above the lag alert threshold indicate the
// dispatch worker is falling behind or stalled.
//
// Call once at worker startup after the repository is available.
func RegisterOldestPendingAgeGauge(meter metric.Meter, repo Repository) error {
	_, err := meter.Float64ObservableGauge(
		"outbox.oldest_pending_age_seconds",
		metric.WithDescription("Age in seconds of the oldest pending-due outbox entry. Zero when the queue is empty."),
		metric.WithFloat64Callback(func(ctx context.Context, obs metric.Float64Observer) error {
			age, err := repo.OldestPendingAgeSecs(ctx)
			if err != nil {
				return nil
			}
			obs.Observe(age)
			return nil
		}),
	)
	return err
}
