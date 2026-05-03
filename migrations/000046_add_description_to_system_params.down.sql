UPDATE system_params SET is_runtime = FALSE WHERE key = 'cache.leaderboard_ttl_seconds';

ALTER TABLE system_params DROP COLUMN IF EXISTS description;
