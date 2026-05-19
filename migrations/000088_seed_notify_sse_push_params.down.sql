DELETE FROM system_params
WHERE key IN (
    'notify.sse_heartbeat_interval_sec',
    'notify.web_push_ttl_sec'
);
