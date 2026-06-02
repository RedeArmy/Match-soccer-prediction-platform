-- Migration 000154: seed audit.max_in_flight system parameter.
--
-- Adds an operator-tunable ceiling on the number of concurrent audit-log
-- goroutines. Log() calls that would push the in-flight count above this
-- value drop the entry immediately (incrementing Dropped()) rather than
-- spawning an unbounded number of goroutines under sustained DB pressure,
-- which would cause memory to grow ~100 MB+ per minute.
--
-- Default: 1 000 goroutines.
--   Each goroutine holds ~8 KB of stack, so 1 000 ≈ 8 MB — well within the
--   process budget and generous enough that this limit is never hit under
--   normal operation. The Prometheus alert (WCQAuditGoroutineBacklog) fires
--   at 500, giving operators time to investigate before entries are dropped.
--
-- is_runtime=FALSE: a process restart is required to pick up a changed value
-- (the limit is read once at startup via ConfigureAuditMaxInFlight).

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'audit.max_in_flight',
    '1000',
    '1000',
    'int',
    'system',
    FALSE,
    'Maximum concurrent audit-log goroutines. Log() calls that would exceed '
    'this limit are dropped immediately to prevent OOM under sustained DB '
    'pressure. The Prometheus alert WCQAuditGoroutineBacklog fires at 500, '
    'giving operators time to react before entries are lost. '
    'Default: 1 000. Valid range: 10–10 000. Restart required.'
)
ON CONFLICT (key) DO NOTHING;
