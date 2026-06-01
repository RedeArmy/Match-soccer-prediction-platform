-- Migration 000153: seed admin.rate_limit_* system parameters.
--
-- Adds a dedicated per-admin-user rate-limit bucket for the /api/v1/admin
-- subrouter. These limits are independent of the general api.rate_limit_*
-- bucket so a single admin running a script loop does not exhaust their shared
-- quota for other endpoints and to constrain expensive bulk operations
-- (POST /admin/system-params/bulk, POST /admin/groups/{id}/distribute-prizes).
--
-- Defaults:
--   admin.rate_limit_rate_per_sec = 2   — 2 admin actions per second
--   admin.rate_limit_burst        = 10  — burst of 10 for occasional batch work
--
-- Both params are is_runtime=FALSE: the LimiterStore is constructed once at
-- API server startup; a process restart is required to apply new values.
-- Valid ranges are enforced by paramIntConstraints (1–100).

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'admin.rate_limit_rate_per_sec',
        '2',
        '2',
        'int',
        'admin',
        FALSE,
        'Per-admin-user token-bucket refill rate (tokens/second) for the '
        '/api/v1/admin subrouter. Independent from api.rate_limit_rate_per_sec. '
        'Default: 2. Valid range: 1–100. Restart required to apply changes.'
    ),
    (
        'admin.rate_limit_burst',
        '10',
        '10',
        'int',
        'admin',
        FALSE,
        'Maximum burst size of the per-admin-user token bucket for '
        '/api/v1/admin routes. A burst of 10 allows comfortable manual panel '
        'use while blocking automated tight loops. Default: 10. '
        'Valid range: 1–100. Restart required to apply changes.'
    )
ON CONFLICT (key) DO NOTHING;
