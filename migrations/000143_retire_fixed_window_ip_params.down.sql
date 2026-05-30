-- Re-inserts the fixed-window IP rate limit params removed in the up migration.
-- This restores the database to the state left by migration 000139, without
-- re-applying the token-bucket params (those are owned by migration 000142).
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'api.ip_global_rate_limit_requests',
        '100', '100', 'int', 'api', FALSE,
        'L1: maximum requests per IP within the global window. Superseded by api.ip_rate_limit_global_rps.'
    ),
    (
        'api.ip_global_rate_limit_window_sec',
        '10', '10', 'int', 'api', FALSE,
        'L1: fixed-window duration in seconds. Superseded by api.ip_rate_limit_global_burst.'
    ),
    (
        'api.ip_webhook_rate_limit_requests',
        '20', '20', 'int', 'api', FALSE,
        'L2: maximum webhook requests per IP per window. Superseded by api.ip_rate_limit_webhook_rps.'
    ),
    (
        'api.ip_webhook_rate_limit_window_sec',
        '60', '60', 'int', 'api', FALSE,
        'L2: fixed-window duration for webhook limiter. Superseded by api.ip_rate_limit_webhook_burst.'
    )
ON CONFLICT (key) DO NOTHING;
