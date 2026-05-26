INSERT INTO system_params (key, value, description, default_value, is_runtime, category)
VALUES (
    'kyc.risk_dashboard_cache_ttl_sec',
    '60',
    'Seconds the admin KYC risk-dashboard response is cached. Lower values increase DB load; upper bound is 1 hour.',
    '60',
    TRUE,
    'kyc'
)
ON CONFLICT (key) DO NOTHING;
