-- Seed notification outbox worker tuning parameters (migration 000111).
--
-- These five params control the behaviour of the outbox dispatch worker that
-- polls domain_outbox and delivers entries to the fan-out dispatcher (SSE,
-- Web Push, Email). All five are is_runtime=FALSE: a worker restart is required
-- for a new value to take effect (the options are applied once in NewWorker).
--
-- notify.outbox_batch_size
--   Maximum domain_outbox entries claimed (and dispatched) per poll cycle.
--   Increase to drain a large backlog faster; decrease to reduce per-cycle DB load.
--
-- notify.outbox_poll_interval_sec
--   Seconds between successive poll cycles. Lower values reduce notification
--   latency at the cost of more idle database round-trips when the queue is empty.
--
-- notify.outbox_lock_duration_sec
--   Seconds a claimed outbox row is held before the stale-lock recovery job
--   reclaims it. Must be longer than the worst-case dispatch time for one entry
--   (including network timeouts to SSE, push, and email providers).
--
-- notify.outbox_max_attempts
--   Maximum dispatch attempts for a single outbox entry before it is permanently
--   marked failed and moved to the dead-letter queue.
--
-- notify.outbox_lag_alert_threshold_sec
--   Outbox lag in seconds above which the worker fires a NotifyOutboxLag alert
--   via the observability notifier on each poll cycle.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.outbox_batch_size',
        '50',
        '50',
        'int',
        'notify',
        FALSE,
        'Maximum domain_outbox entries claimed and dispatched per poll cycle by the outbox worker. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.outbox_poll_interval_sec',
        '2',
        '2',
        'int',
        'notify',
        FALSE,
        'Seconds between outbox poll cycles. Lower values reduce notification latency at the cost of more idle DB round-trips. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.outbox_lock_duration_sec',
        '300',
        '300',
        'int',
        'notify',
        FALSE,
        'Seconds a claimed outbox row is held before the stale-lock recovery job reclaims it. Must exceed the worst-case dispatch time per entry. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.outbox_max_attempts',
        '5',
        '5',
        'int',
        'notify',
        FALSE,
        'Maximum dispatch attempts for a single outbox entry before it is permanently marked failed. is_runtime=FALSE: worker restart required.'
    ),
    (
        'notify.outbox_lag_alert_threshold_sec',
        '30',
        '30',
        'int',
        'notify',
        FALSE,
        'Outbox lag in seconds above which the worker fires a NotifyOutboxLag alert on each poll cycle. is_runtime=FALSE: worker restart required.'
    )
ON CONFLICT (key) DO NOTHING;
