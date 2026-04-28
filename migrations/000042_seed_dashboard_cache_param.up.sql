-- Add cache.dashboard_ttl_seconds to system_params.
--
-- Controls how long GetDashboardStats results are cached in-process. A value
-- of 30 absorbs repeated dashboard loads without the aggregate counts becoming
-- stale enough to mislead administrators. Set to 0 to disable caching entirely.
-- is_runtime = TRUE so the TTL can be tuned or disabled without a restart.

INSERT INTO system_params (key, value, type, category, is_runtime)
VALUES ('cache.dashboard_ttl_seconds', '30', 'int', 'cache', TRUE)
ON CONFLICT (key) DO NOTHING;
