-- Reverse of migration 000160: remove the three params seeded by the up migration.
DELETE FROM system_params
WHERE key IN (
    'notify.sse_evict_after_drops',
    'worker.leaderboard_publish_max_attempts',
    'worker.leaderboard_publish_base_delay_ms'
);
