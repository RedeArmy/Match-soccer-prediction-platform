DELETE FROM system_params WHERE key = 'auth.session_max_age_seconds';
DROP TABLE IF EXISTS revoked_sessions;
