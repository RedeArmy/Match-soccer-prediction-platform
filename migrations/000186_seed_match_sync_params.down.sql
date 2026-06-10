DELETE FROM system_params
WHERE key IN (
    'match.sync.enabled',
    'match.sync.fast_poll_interval_sec',
    'match.sync.slow_poll_interval_sec',
    'match.sync.provider',
    'match.sync.league_id',
    'match.sync.season'
);
