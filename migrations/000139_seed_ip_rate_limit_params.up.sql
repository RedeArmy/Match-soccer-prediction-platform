-- Seeds IP-based rate limit parameters for the two new defence layers:
--
--   L1 (global per-IP): api.ip_global_rate_limit_requests /
--   api.ip_global_rate_limit_window_sec — applied to every route including
--   /health and /webhooks/*. 100 requests per 10-second window is generous
--   for real users behind shared NAT while blocking volumetric attacks.
--
--   L2 (webhook per-IP): api.ip_webhook_rate_limit_requests /
--   api.ip_webhook_rate_limit_window_sec — applied only to /webhooks/* routes.
--   20 requests per 60-second window protects the CPU-expensive PayPal RSA
--   signature verification from webhook replay floods.
--
-- is_runtime=FALSE for all four: the IPRateLimiter is configured once at
-- process startup; a restart is required to apply changed values.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'api.ip_global_rate_limit_requests',
        '100',
        '100',
        'int',
        'api',
        FALSE,
        'L1: maximum requests per IP within the global window (api.ip_global_rate_limit_window_sec). '
        'Applies to every route. Restart required to change.'
    ),
    (
        'api.ip_global_rate_limit_window_sec',
        '10',
        '10',
        'int',
        'api',
        FALSE,
        'L1: fixed-window duration in seconds for the global per-IP rate limiter. Restart required.'
    ),
    (
        'api.ip_webhook_rate_limit_requests',
        '20',
        '20',
        'int',
        'api',
        FALSE,
        'L2: maximum requests per IP within the webhook window (api.ip_webhook_rate_limit_window_sec). '
        'Applies only to /webhooks/* routes. Restart required to change.'
    ),
    (
        'api.ip_webhook_rate_limit_window_sec',
        '60',
        '60',
        'int',
        'api',
        FALSE,
        'L2: fixed-window duration in seconds for the webhook per-IP rate limiter. Restart required.'
    )
ON CONFLICT (key) DO NOTHING;
