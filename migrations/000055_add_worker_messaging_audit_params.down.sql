-- Rollback: remove the 10 system_params rows added by the up migration.
-- The worker, api, and extended messaging/system rows are deleted by key.
-- Rows that pre-existed this migration (e.g. messaging.max_retries) are
-- NOT affected; only the keys introduced by 000055 are removed.

DELETE FROM system_params
WHERE key IN (
    'worker.snapshot_concurrency',
    'worker.snapshot_retry_base_ms',
    'worker.snapshot_max_attempts',
    'worker.dlq_monitor_interval_sec',
    'worker.purge_interval_hours',
    'messaging.stream_worker_count',
    'messaging.stream_read_block_sec',
    'audit.max_retries',
    'audit.retry_delay_ms',
    'api.body_size_limit_bytes'
);
