-- Seed notification DLQ replay worker tuning parameters (migration 000110).
--
-- These four params control the behaviour of the DLQ replay worker that polls
-- notification_dlq and re-inserts eligible entries into domain_outbox.
-- All four are is_runtime=FALSE: a worker restart is required for a new value
-- to take effect (the options are applied once in the NewDLQWorker constructor).
--
-- notify.dlq_replay_batch_size
--   Maximum notification_dlq entries claimed (and re-inserted) per poll cycle.
--   Increase to drain a large backlog faster; decrease to reduce per-cycle DB load.
--
-- notify.dlq_replay_poll_interval_sec
--   Seconds between successive poll cycles. The worker sleeps this long between
--   ClaimBatch calls. Lower values recover from failures sooner at the cost of
--   more idle DB round-trips when the queue is empty.
--
-- notify.dlq_replay_max_attempts
--   Maximum replay attempts allowed for a single entry before it is permanently
--   abandoned (resolved_at stays NULL, attempts = max). Entries beyond this
--   threshold are not re-claimed by ClaimBatch.
--
-- notify.dlq_replay_alert_threshold
--   Unresolved entry count above which the DLQ worker fires an n8n overflow
--   alert (via ObservabilityNotifier.NotifyDLQOverflow) on each poll cycle.
--   Set higher during planned maintenance to suppress expected queue growth.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.dlq_replay_batch_size',
        '20',
        '20',
        'int',
        'notify',
        FALSE,
        'Maximum notification_dlq entries claimed and replayed per poll cycle by the DLQ replay worker. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.dlq_replay_poll_interval_sec',
        '30',
        '30',
        'int',
        'notify',
        FALSE,
        'Seconds between DLQ replay poll cycles. Lower values recover from notification failures faster at the cost of more idle DB round-trips. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.dlq_replay_max_attempts',
        '5',
        '5',
        'int',
        'notify',
        FALSE,
        'Maximum replay attempts for a single notification_dlq entry before it is permanently abandoned. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.dlq_replay_alert_threshold',
        '50',
        '50',
        'int',
        'notify',
        FALSE,
        'Unresolved notification_dlq entry count above which the DLQ replay worker fires an n8n overflow alert on each poll cycle. is_runtime=FALSE: worker restart required.'
    )
ON CONFLICT (key) DO NOTHING;
