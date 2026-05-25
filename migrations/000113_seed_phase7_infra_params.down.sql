-- Rollback: remove Phase 7 infrastructure tuning parameters (migration 000113).
DELETE FROM system_params WHERE key IN (
    'notify.sse_chan_buf_size',
    'notify.outbox_stale_lock_threshold_sec'
);
