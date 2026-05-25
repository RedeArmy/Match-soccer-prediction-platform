DELETE FROM system_params WHERE key IN (
    'breaker.cache_max_fails',
    'breaker.cache_cooldown_sec'
);
