-- Partial index to accelerate AML velocity checks.
--
-- SumTransactionsByUserAndPeriod and ExceedsAMLThreshold both filter
-- balance_ledger by user_id, created_at >= (now - 24h), and kind IN
-- ('bank_transfer','webhook_recurrente','webhook_paypal','prize').
-- Without this index the query does a full user-partition scan for every
-- deposit event; with it, Postgres uses an index-only scan bounded to the
-- rolling window.
--
-- Partitioning plan (ADR docs/adr/0002-balance-ledger-partitioning.md):
-- When the table exceeds ~50M rows, convert to range-partitioned by
-- created_at (monthly or quarterly).  This index becomes a local index on
-- each partition, so the same query shape continues to use it without
-- modification.  The ADR captures the decision criteria and cut-over steps.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_balance_ledger_velocity
    ON balance_ledger (user_id, created_at DESC)
    WHERE kind IN ('bank_transfer', 'webhook_recurrente', 'webhook_paypal', 'prize');
