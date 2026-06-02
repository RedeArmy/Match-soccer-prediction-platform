DELETE FROM system_params
WHERE key IN (
    'fx.banguat_timeout_sec',
    'fx.exchange_rate_api_timeout_sec',
    'fx.open_exchange_timeout_sec'
);
