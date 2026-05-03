-- Add human-readable description column to system_params so operators can
-- understand each parameter's purpose without consulting source code.
-- Also corrects cache.leaderboard_ttl_seconds is_runtime to TRUE: the mutation
-- hook added in service layer now propagates value changes immediately without
-- requiring a process restart.

ALTER TABLE system_params
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

UPDATE system_params SET description = 'Points awarded for predicting the exact final score'
    WHERE key = 'scoring.exact_score';
UPDATE system_params SET description = 'Points awarded for predicting the correct match outcome (win, draw, or loss)'
    WHERE key = 'scoring.correct_outcome';
UPDATE system_params SET description = 'Points awarded for predicting the correct goal difference'
    WHERE key = 'scoring.goal_difference';
UPDATE system_params SET description = 'Minutes before kick-off after which new or updated predictions are rejected'
    WHERE key = 'prediction.deadline_minutes';
UPDATE system_params SET description = 'Minimum number of paid members required to activate a quiniela group'
    WHERE key = 'group.min_members_for_active';
UPDATE system_params SET description = 'Default minimum number of paid members required for prize eligibility'
    WHERE key = 'group.default_prize_threshold';
UPDATE system_params SET description = 'Number of characters in a randomly generated group invite code'
    WHERE key = 'group.invite_code_length';
UPDATE system_params SET description = 'Age in days after which a pending payment or membership is flagged as a conflict'
    WHERE key = 'conflict.stale_days';
UPDATE system_params SET description = 'Default number of items per page for paginated admin endpoints'
    WHERE key = 'pagination.default_limit';
UPDATE system_params SET description = 'Maximum number of items per page allowed by paginated endpoints'
    WHERE key = 'pagination.max_limit';
UPDATE system_params SET description = 'Standing points awarded for a group-stage win'
    WHERE key = 'tournament.win_points';
UPDATE system_params SET description = 'Maximum number of IDs accepted in a single bulk admin operation'
    WHERE key = 'admin.bulk_max_items';
UPDATE system_params SET description = 'Match-list cache TTL in seconds; restart required to apply changes'
    WHERE key = 'cache.match_ttl_seconds';
UPDATE system_params SET description = 'Leaderboard cache TTL in seconds; applied immediately via mutation hook without restart'
    WHERE key = 'cache.leaderboard_ttl_seconds';
UPDATE system_params SET description = 'Dashboard stats cache TTL in seconds; set to 0 to disable the cache'
    WHERE key = 'cache.dashboard_ttl_seconds';
UPDATE system_params SET description = 'Maximum seconds the audit log goroutine waits to persist an entry before giving up'
    WHERE key = 'audit.write_timeout_seconds';
UPDATE system_params SET description = 'Maximum number of dead-letter queue entries returned in the Stats sample'
    WHERE key = 'dlq.sample_size';
UPDATE system_params SET description = 'Default number of DLQ entries replayed when no explicit limit is supplied'
    WHERE key = 'dlq.replay_default_limit';
UPDATE system_params SET description = 'Total handler attempts before an event is moved to the dead-letter queue'
    WHERE key = 'messaging.max_retries';
UPDATE system_params SET description = 'Approximate maximum length of the Redis event stream (MAXLEN ~)'
    WHERE key = 'messaging.stream_max_len';
UPDATE system_params SET description = 'JWKS validation timeout in seconds at process startup'
    WHERE key = 'auth.validation_timeout_seconds';

-- Reflect that cache.leaderboard_ttl_seconds is now runtime-tunable via the
-- mutation hook registered in server.go buildHandlers.
UPDATE system_params SET is_runtime = TRUE WHERE key = 'cache.leaderboard_ttl_seconds';
