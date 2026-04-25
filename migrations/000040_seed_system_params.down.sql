DELETE FROM system_params
WHERE key IN (
    'scoring.exact_score',
    'scoring.correct_outcome',
    'scoring.goal_difference',
    'prediction.deadline_minutes',
    'group.min_members_for_active',
    'group.default_prize_threshold',
    'group.invite_code_length',
    'conflict.stale_days',
    'pagination.default_limit',
    'pagination.max_limit',
    'tournament.win_points',
    'audit.write_timeout_seconds',
    'auth.validation_timeout_seconds',
    'cache.match_ttl_seconds',
    'cache.leaderboard_ttl_seconds',
    'dlq.sample_size',
    'dlq.replay_default_limit',
    'messaging.max_retries',
    'messaging.stream_max_len'
);
