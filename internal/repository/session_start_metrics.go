package repository

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// sessionStartMetrics bundles the OTel instruments used by
// PostgresSessionStartRepository. All fields are nil until
// RegisterSessionStartMetrics is called.
type sessionStartMetrics struct {
	upsertDuration metric.Float64Histogram
	upsertTotal    metric.Int64Counter
	rowCount       metric.Int64ObservableGauge
}

// RegisterSessionStartMetrics wires three OTel instruments into the repository:
//
//   - wcq_session_starts_upsert_duration_seconds: latency histogram for each
//     UpsertSessionStart DB call. Useful for detecting index bloat or lock
//     contention on the session_starts table.
//
//   - wcq_session_starts_upsert_total: cumulative count of UpsertSessionStart
//     calls. Because this method is only called for OAuth / fva-absent sessions,
//     the counter indirectly measures the fraction of OAuth traffic. A sustained
//     increase relative to total authenticated requests signals more OAuth users.
//
//   - wcq_session_starts_row_count: estimated table size via pg_class.reltuples,
//     sampled on each Prometheus scrape. Grows as new OAuth sessions arrive and
//     shrinks when the daily session.starts_prune job runs. An unexpectedly large
//     value indicates the prune job is failing or max-age is set very high.
//
// Call once at startup after the global meter provider is initialised. Safe to
// skip in tests — nil instruments are no-ops in UpsertSessionStart.
func (r *PostgresSessionStartRepository) RegisterSessionStartMetrics(meter metric.Meter) error {
	h, err := meter.Float64Histogram(
		"wcq_session_starts_upsert_duration_seconds",
		metric.WithDescription(
			"Latency of UpsertSessionStart DB calls in seconds. "+
				"Called once per OAuth session per replica (subsequent requests use the in-process cache). "+
				"Elevated p99 indicates table bloat or lock contention on session_starts.",
		),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5),
	)
	if err != nil {
		return err
	}

	c, err := meter.Int64Counter(
		"wcq_session_starts_upsert_total",
		metric.WithDescription(
			"Total UpsertSessionStart calls. "+
				"Each call corresponds to a new OAuth session (or the first request from an "+
				"existing OAuth session to this replica). "+
				"Divide by total authenticated requests to estimate the OAuth traffic fraction.",
		),
	)
	if err != nil {
		return err
	}

	g, err := meter.Int64ObservableGauge(
		"wcq_session_starts_row_count",
		metric.WithDescription(
			"Estimated number of rows in the session_starts table (pg_class.reltuples). "+
				"Grows with new OAuth sessions; shrinks after the daily session.starts_prune job. "+
				"Unexpectedly large values indicate prune failures or very high max-age settings.",
		),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			n, err := r.countRows(ctx)
			if err != nil {
				return nil // swallow; don't fail the collection cycle
			}
			obs.Observe(n)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	r.metrics = &sessionStartMetrics{
		upsertDuration: h,
		upsertTotal:    c,
		rowCount:       g,
	}
	return nil
}

// countRows returns the estimated row count for session_starts using
// pg_class.reltuples (updated by ANALYZE / autovacuum). Matches the pattern
// used by RegisterLedgerRowCountGauge for balance_ledger.
func (r *PostgresSessionStartRepository) countRows(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx,
		`SELECT reltuples::bigint FROM pg_class WHERE relname = 'session_starts'`,
	).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// recordUpsert records latency and increments the call counter. No-op when
// metrics are not registered (nil check guards both fields via the parent struct).
func (r *PostgresSessionStartRepository) recordUpsert(ctx context.Context, start time.Time) {
	if r.metrics == nil {
		return
	}
	elapsed := time.Since(start).Seconds()
	r.metrics.upsertDuration.Record(ctx, elapsed)
	r.metrics.upsertTotal.Add(ctx, 1)
}
