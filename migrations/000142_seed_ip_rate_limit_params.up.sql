-- Seeds the four system parameters that govern the two-tier IP-based rate
-- limiter (internal/middleware/rate_limit_ip.go).
--
-- L1 global (api.ip_rate_limit_global_rps / _global_burst):
--   One token-bucket per source IP applied to all /api/v1 routes.
--   50 RPS / 100 burst is permissive for legitimate multi-tab users while
--   blocking sustained bot scans from a single address.
--
-- L2 webhook (api.ip_rate_limit_webhook_rps / _webhook_burst):
--   Tighter bucket applied only to /webhooks/recurrente and /webhooks/paypal.
--   Legitimate webhook senders (Recurrente, PayPal) rarely exceed 1 req/s per
--   source; 5 RPS / 10 burst stops replay attacks without blocking providers.
--
-- is_runtime=FALSE: the LimiterStore is constructed once at API server startup;
-- a process restart is required for new values to take effect.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'api.ip_rate_limit_global_rps',
        '50', '50',
        'int', 'api',
        FALSE,
        'Token-bucket refill rate (tokens/second) for the L1 per-IP rate limiter applied to all /api/v1 routes. One bucket is maintained per source IP. Default: 50 RPS. is_runtime=FALSE: API server restart required.'
    ),
    (
        'api.ip_rate_limit_global_burst',
        '100', '100',
        'int', 'api',
        FALSE,
        'Maximum burst size for the L1 per-IP global token bucket. Allows short activity spikes (e.g. page load with parallel requests) while the refill rate caps sustained throughput. Default: 100. is_runtime=FALSE: restart required.'
    ),
    (
        'api.ip_rate_limit_webhook_rps',
        '5', '5',
        'int', 'api',
        FALSE,
        'Token-bucket refill rate (tokens/second) for the L2 per-IP rate limiter applied only to /webhooks/recurrente and /webhooks/paypal. Legitimate providers rarely exceed 1 req/s; this threshold stops replay attacks without blocking real deliveries. Default: 5 RPS. is_runtime=FALSE: restart required.'
    ),
    (
        'api.ip_rate_limit_webhook_burst',
        '10', '10',
        'int', 'api',
        FALSE,
        'Maximum burst size for the L2 per-IP webhook token bucket. Allows brief delivery bursts (e.g. PayPal retrying a failed webhook batch) while keeping the sustained rate low. Default: 10. is_runtime=FALSE: restart required.'
    )
ON CONFLICT (key) DO NOTHING;
