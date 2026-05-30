-- Removes the four fixed-window IP rate limit system_params seeded by migration
-- 000139 that were superseded by the token-bucket params in migration 000142.
--
-- Migration 000139 used a fixed-window model (requests / window_sec):
--   api.ip_global_rate_limit_requests
--   api.ip_global_rate_limit_window_sec
--   api.ip_webhook_rate_limit_requests
--   api.ip_webhook_rate_limit_window_sec
--
-- Migration 000142 replaced these with a token-bucket model (rps / burst):
--   api.ip_rate_limit_global_rps
--   api.ip_rate_limit_global_burst
--   api.ip_rate_limit_webhook_rps
--   api.ip_rate_limit_webhook_burst
--
-- The fixed-window params are not referenced by any application code or domain
-- constant; keeping them causes validate-params to emit UNEXPECTED PARAM IN DB
-- warnings on every run. Removing them brings the DB into sync with the domain
-- constants and eliminates the spurious warnings.
--
-- This migration is safe to run while the API is live: system_params rows are
-- not foreign-key-referenced by any other table, and the application has
-- already been reading the new token-bucket keys since 000142 was applied.

DELETE FROM system_params
WHERE key IN (
    'api.ip_global_rate_limit_requests',
    'api.ip_global_rate_limit_window_sec',
    'api.ip_webhook_rate_limit_requests',
    'api.ip_webhook_rate_limit_window_sec'
);
