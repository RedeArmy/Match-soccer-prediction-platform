package repository

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

// LedgerRowCounter is the narrow read interface consumed by
// RegisterLedgerRowCountGauge.  It is satisfied by
// *PostgresBalanceLedgerRepository.
type LedgerRowCounter interface {
	CountRows(ctx context.Context) (int64, error)
}

// RegisterLedgerRowCountGauge registers an observable gauge that reports the
// estimated number of rows in balance_ledger using pg_class.reltuples.
//
// The gauge is sampled on each Prometheus scrape interval (typically 15 s).
// A sustained increase toward the partitioning threshold (documented in
// docs/adr/0008-balance-ledger-partitioning.md) triggers the
// WCQLedgerRowCountWarning alert, giving the operations team advance notice
// to execute the zero-downtime migration before query performance degrades.
//
// Call once at worker startup after the repository is available.
func RegisterLedgerRowCountGauge(meter metric.Meter, repo LedgerRowCounter) error {
	_, err := meter.Int64ObservableGauge(
		"wcq_balance_ledger_row_count",
		metric.WithDescription(
			"Estimated number of rows in the balance_ledger table (pg_class.reltuples). "+
				"Alert threshold: 2M (warning), 10M (action required). "+
				"Partitioning plan: docs/adr/0008-balance-ledger-partitioning.md.",
		),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			n, err := repo.CountRows(ctx)
			if err != nil {
				return nil // swallow; don't fail the collection cycle
			}
			obs.Observe(n)
			return nil
		}),
	)
	return err
}
