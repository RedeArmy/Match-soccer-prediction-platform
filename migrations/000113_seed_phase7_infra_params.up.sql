-- Migration 000113: seed Phase 7 infrastructure tuning parameters
--
-- Two params introduced in Phase 7 to make previously hardcoded values
-- configurable via the admin API without a code change:
--
--   notify.sse_chan_buf_size
--     Per-connection channel buffer size used by the SSE hub. Larger values
--     tolerate bursty event rates at the cost of additional heap per connection.
--     is_runtime=FALSE: API server restart required (applied once at Hub
--     construction inside Routes()).
--
--   notify.outbox_stale_lock_threshold_sec
--     Seconds a domain_outbox row may remain in 'processing' status past its
--     locked_until timestamp before the stale-lock recovery job reclaims it.
--     Must exceed the worst-case dispatch time for a single entry (including
--     network retries to SSE, push, and email providers).
--     is_runtime=FALSE: worker restart required (applied once in
--     NewPostgresRepository via the WithStaleLockThreshold option).

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.sse_chan_buf_size',
        '64',
        '64',
        'int',
        'notify',
        FALSE,
        'Per-connection SSE channel buffer size. Increase to absorb event bursts; each extra slot costs one Notification struct on the heap per open connection. is_runtime=FALSE: API server restart required.'
    ),
    (
        'notify.outbox_stale_lock_threshold_sec',
        '600',
        '600',
        'int',
        'notify',
        FALSE,
        'Seconds a domain_outbox row may remain in processing past locked_until before the recovery job reclaims it. Must exceed the worst-case per-entry dispatch time. is_runtime=FALSE: worker restart required.'
    )
ON CONFLICT (key) DO NOTHING;
