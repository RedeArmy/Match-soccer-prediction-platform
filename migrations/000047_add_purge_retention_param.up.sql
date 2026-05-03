INSERT INTO system_params (key, value, type, category, is_runtime, description)
VALUES (
    'system.purge_retention_days',
    '30',
    'int',
    'system',
    false,
    'Age in days after which soft-deleted users and quinielas are permanently removed from the database by the worker purge goroutine'
)
ON CONFLICT (key) DO NOTHING;
