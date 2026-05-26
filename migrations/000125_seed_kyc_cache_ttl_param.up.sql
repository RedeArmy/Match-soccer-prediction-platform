INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'kyc.risk_dashboard_cache_ttl_sec',
    '60',
    '60',
    'int',
    'kyc',
    TRUE,
    'Seconds the admin KYC risk-dashboard response is cached. Lower values increase DB load; upper bound is 1 hour.'
)
ON CONFLICT (key) DO NOTHING;
