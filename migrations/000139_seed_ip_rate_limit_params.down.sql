DELETE FROM system_params WHERE key IN (
    'api.ip_global_rate_limit_requests',
    'api.ip_global_rate_limit_window_sec',
    'api.ip_webhook_rate_limit_requests',
    'api.ip_webhook_rate_limit_window_sec'
);
