-- Rollback: remove notification outbox worker params (migration 000111).
DELETE FROM system_params WHERE key IN (
    'notify.outbox_batch_size',
    'notify.outbox_poll_interval_sec',
    'notify.outbox_lock_duration_sec',
    'notify.outbox_max_attempts',
    'notify.outbox_lag_alert_threshold_sec'
);
