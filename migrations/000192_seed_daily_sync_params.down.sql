DELETE FROM system_params
WHERE key IN (
    'match.dailysync.hour',
    'match.sync.prematch_window_min',
    'match.sync.stop_after_zero_live_count'
);
