-- Migration 000158: seed worker.outbox_retention_days system parameter.
--
-- domain_outbox rows in terminal state (done/failed) are operationally inert:
-- they will never be retried or replayed.  Without a purge policy the table
-- grows unboundedly and periodic VACUUM must reclaim an ever-increasing number
-- of dead tuples.
--
-- This parameter controls how many days terminal-state outbox rows are kept
-- before the daily purge job deletes them.  The purge uses the partial index
-- idx_outbox_completed_created_at (migration 000157) for an efficient range scan.
--
-- Default: 30 days.
--   Covers the full FIFA 2026 tournament window and provides a month of
--   operational history for debugging notification delivery issues.
--   At peak load (50K DAU × 4 events/match × 64 matches) the table holds
--   ≈12.8M terminal rows before purge; 30-day retention bounds steady-state
--   size to daily_events × 30.
--
-- is_runtime=FALSE: a worker restart is required to pick up a new value.
-- Valid range: 1–365 (enforced by paramIntConstraints).

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'worker.outbox_retention_days',
    '30',
    '30',
    'int',
    'worker',
    FALSE,
    'Number of days terminal-state (done/failed) domain_outbox rows are retained. '
    'Rows whose created_at is strictly before (NOW() - retention) are deleted by '
    'the daily purge job. Default: 30 days. Valid range: 1–365. '
    'Worker restart required to apply changes.'
)
ON CONFLICT (key) DO NOTHING;
