DELETE FROM system_params WHERE key IN (
    'api.ip_rate_limit_global_rps',
    'api.ip_rate_limit_global_burst',
    'api.ip_rate_limit_webhook_rps',
    'api.ip_rate_limit_webhook_burst'
);
