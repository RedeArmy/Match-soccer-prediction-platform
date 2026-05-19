INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.scheduler_timezone',
    'America/Guatemala',
    'America/Guatemala',
    'string',
    'notify',
    FALSE,
    'IANA timezone used by the notification scheduler when evaluating daily (08:00) and weekly (Monday 08:00) job schedules. Requires a worker restart to take effect.'
)
ON CONFLICT (key) DO NOTHING;
