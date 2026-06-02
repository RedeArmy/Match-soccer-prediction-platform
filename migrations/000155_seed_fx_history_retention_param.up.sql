-- Migration 000155: seed fx.history_retention_days system parameter.
--
-- exchange_rate_history grows by one row per RefreshRate or OverrideRate call.
-- At the normal daily refresh cadence that is 365 rows/year, but if an operator
-- calls /admin/exchange-rate/refresh manually the table grows faster. Without a
-- purge policy the table is unbounded.
--
-- This parameter controls how many days of history the daily purge job retains.
-- Rows whose effective_at is strictly before (NOW() - retention) are deleted.
--
-- Default: 90 days.
--   Covers the FIFA 2026 tournament window (≈ 30 days) plus two months of
--   post-tournament analysis. At 1 refresh/day this caps the table at ~90 rows;
--   at 10 overrides/day over 90 days it caps at ~900 rows — both negligible.
--
-- is_runtime=FALSE: a worker restart is required to pick up a new value.
-- Valid range: 7–730 (enforced by paramIntConstraints).

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'fx.history_retention_days',
    '90',
    '90',
    'int',
    'fx',
    FALSE,
    'Number of days exchange_rate_history rows are retained. Rows whose '
    'effective_at is strictly before (NOW() - retention) are deleted by the '
    'daily purge job. Default: 90 days. Valid range: 7–730. '
    'Worker restart required to apply changes.'
)
ON CONFLICT (key) DO NOTHING;
