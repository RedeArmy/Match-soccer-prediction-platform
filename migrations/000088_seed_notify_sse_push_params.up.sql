-- Seed system_params for SSE heartbeat interval and Web Push TTL (Phase 2).
-- Both parameters are is_runtime=TRUE; changes propagate within the 30 s cache
-- window without a process restart.  Idempotent: ON CONFLICT DO NOTHING.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.sse_heartbeat_interval_sec',
        '30', '30',
        'int', 'notify',
        TRUE,
        'SSE keepalive heartbeat interval in seconds. Increase to reduce idle connections; decrease for faster client disconnect detection. Default: 30. Changeable at runtime.'
    ),
    (
        'notify.web_push_ttl_sec',
        '86400', '86400',
        'int', 'notify',
        TRUE,
        'Web Push message TTL in seconds passed to the push service. Messages older than this are dropped if undelivered. Default: 86400 (24 hours). Changeable at runtime.'
    )
ON CONFLICT (key) DO NOTHING;
