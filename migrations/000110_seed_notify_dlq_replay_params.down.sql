-- Rollback: remove notification DLQ replay worker params (migration 000110).
DELETE FROM system_params WHERE key IN (
    'notify.dlq_replay_batch_size',
    'notify.dlq_replay_poll_interval_sec',
    'notify.dlq_replay_max_attempts',
    'notify.dlq_replay_alert_threshold'
);
