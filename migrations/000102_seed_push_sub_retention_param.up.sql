-- Migration 000102: seed system parameter for push subscription pruning retention.
--
-- notify.push_sub_retention_days — number of days after a push subscription is
--   marked inactive before the daily pruning job permanently deletes the row.
--   Uses COALESCE(inactivated_at, created_at) so rows inactivated before
--   migration 000101 are pruned based on their created_at timestamp.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.push_sub_retention_days',
    '30', '30',
    'int', 'notify',
    TRUE,
    'Days after inactivation before a push subscription row is deleted by the daily pruning job (1–365).'
)
ON CONFLICT (key) DO NOTHING;
