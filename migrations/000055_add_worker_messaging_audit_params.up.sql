-- Migration: add worker, messaging, audit, and API system parameters
--
-- Adds 10 new system_params rows covering operational constants that were
-- previously hardcoded. All new params carry is_runtime=FALSE because the
-- values are read once at process startup and require a restart to take effect.
--
-- Categories introduced:
--   worker  - leaderboard snapshot generation and background maintenance jobs
--   api     - HTTP server request handling limits
--
-- Existing categories extended:
--   messaging - Redis Streams consumer pool configuration
--   system    - audit retry policy (consistent with audit.write_timeout_seconds)

INSERT INTO system_params (key, value, type, category, is_runtime, description) VALUES

    -- Worker: leaderboard snapshot generation
    ('worker.snapshot_concurrency',   '16',  'int', 'worker', FALSE,
     'Maximum concurrent quiniela leaderboard snapshots per MatchFinished event; higher values reduce wall-clock latency at the cost of more concurrent DB connections'),

    ('worker.snapshot_retry_base_ms', '100', 'int', 'worker', FALSE,
     'Initial backoff in milliseconds between snapshot write retries; doubles on each subsequent attempt (100ms → 200ms → 400ms)'),

    ('worker.snapshot_max_attempts',  '3',   'int', 'worker', FALSE,
     'Maximum snapshot write attempts per quiniela per match event; on exhaustion the failure is logged at Warn and processing continues'),

    -- Worker: background maintenance jobs
    ('worker.dlq_monitor_interval_sec', '300', 'int', 'worker', FALSE,
     'Seconds between dead-letter queue size log events; operators should alert on non-zero DLQ counts within this window'),

    ('worker.purge_interval_hours',     '24',  'int', 'worker', FALSE,
     'Hours between purge runs that permanently remove soft-deleted users and quinielas older than system.purge_retention_days'),

    -- Messaging: Redis Streams consumer pool
    ('messaging.stream_worker_count',   '8',   'int', 'messaging', FALSE,
     'Goroutines in the per-EventType worker pool; events for different matches are processed concurrently up to this limit'),

    ('messaging.stream_read_block_sec', '5',   'int', 'messaging', FALSE,
     'XREADGROUP block timeout in seconds; a smaller value makes the consumer loop react faster to shutdown signals at the cost of more idle Redis round-trips'),

    -- Audit retry policy (consistent with audit.write_timeout_seconds category)
    ('audit.max_retries',    '2',   'int', 'system', FALSE,
     'Maximum write attempts for an audit log entry before it is permanently lost; emits a structured audit_lost=true log event on exhaustion'),

    ('audit.retry_delay_ms', '250', 'int', 'system', FALSE,
     'Milliseconds to wait between audit log write retries to allow transient DB failures to clear'),

    -- API: HTTP request limits
    ('api.body_size_limit_bytes', '65536', 'int', 'api', FALSE,
     'Maximum request body size in bytes; requests exceeding this limit are rejected with 413 Content Too Large to prevent DoS via oversized payloads')

ON CONFLICT (key) DO UPDATE SET
    value       = EXCLUDED.value,
    type        = EXCLUDED.type,
    category    = EXCLUDED.category,
    is_runtime  = EXCLUDED.is_runtime,
    description = EXCLUDED.description,
    updated_at  = NOW();
