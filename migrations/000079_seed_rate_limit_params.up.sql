-- Migration: seed api.rate_limit_rate_per_sec and api.rate_limit_burst system parameters
--
-- These params control the per-user token-bucket rate limiter applied to every
-- authenticated request on the /api/v1 subrouter.
--
-- api.rate_limit_rate_per_sec — token refill rate (tokens/second).
--   10 tokens/s gives 600 requests/minute under steady-state load. The bucket
--   refills continuously, so short idle periods quickly restore capacity.
--
-- api.rate_limit_burst — maximum burst size.
--   A burst of 30 allows a user to fire up to 30 back-to-back requests (e.g.
--   loading a dashboard that issues several parallel API calls) before the
--   steady-state rate takes effect.
--
-- Both params are is_runtime=FALSE: the LimiterStore is constructed once when
-- Routes() is called. A process restart is required to apply changed values.
-- Changing them at runtime without a restart has no effect.
--
-- Idempotent: ON CONFLICT DO NOTHING means re-running this migration is safe.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'api.rate_limit_rate_per_sec',
        '10', '10',
        'int', 'api',
        FALSE,
        'Token-bucket refill rate in tokens per second for the per-user API rate limiter on /api/v1. A restart is required for changes to take effect.'
    ),
    (
        'api.rate_limit_burst',
        '30', '30',
        'int', 'api',
        FALSE,
        'Maximum burst size of the per-user token bucket on /api/v1. Allows short back-to-back request spikes before the steady-state rate applies. A restart is required for changes to take effect.'
    )
ON CONFLICT (key) DO NOTHING;
