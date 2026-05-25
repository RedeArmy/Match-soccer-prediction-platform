-- Migration 000115: seed system.param_history_retention_days system parameter.
--
-- This param controls how many days the daily purge job retains rows in
-- system_params_history before hard-deleting them.  It was defined as
-- ParamKeySystemParamHistoryRetentionDays (constants_worker.go) and seeded into
-- AllParamKeys() but was never inserted into system_params.
--
-- Without this row, the worker falls back to the hardcoded default (90 days) but
-- the param is invisible in the admin panel and cannot be tuned at runtime.
-- Adding it here makes the retention window operator-configurable without a code
-- deploy (note: a worker restart is still required because the param is read once
-- at startup).
--
-- is_runtime=FALSE: the purge goroutine reads this once in main.go at startup;
-- changing the value requires a worker restart to take effect.
--
-- Idempotent: ON CONFLICT DO NOTHING means re-running this migration is safe.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'system.param_history_retention_days',
    '90',
    '90',
    'int',
    'system',
    FALSE,
    'Days to retain rows in system_params_history before the daily purge job hard-deletes them. Bounds audit-trail storage for param mutations. Default: 90 days. Worker restart required to apply changes.'
)
ON CONFLICT (key) DO NOTHING;
