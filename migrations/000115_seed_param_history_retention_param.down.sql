-- Rollback: remove the system.param_history_retention_days row added in 000115.
-- Only deletes the row when its value matches the seeded default (90) to avoid
-- removing an operator override.
DELETE FROM system_params
WHERE key = 'system.param_history_retention_days'
  AND value = '90';
