-- Migration 000160: seed SSE eviction and leaderboard broadcaster retry params.
--
-- Promotes three previously hardcoded operational constants to system_params:
--
--   notify.sse_evict_after_drops          (was hub.evictAfterDrops = 5)
--   worker.leaderboard_publish_max_attempts (was publishMaxAttempts = 3)
--   worker.leaderboard_publish_base_delay_ms (was publishBaseDelay = 50 ms)
--
-- All three are is_runtime=FALSE: the hub is built once at server startup
-- (Routes()), and the broadcaster is constructed once at worker startup.
-- A process restart is required to apply changed values.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.sse_evict_after_drops',
        '5',
        '5',
        'int',
        'notify',
        FALSE,
        'Consecutive failed sends on a single SSE connection before the hub evicts it. Higher values tolerate slower consumers at the cost of holding dead connections longer. Requires API restart.'
    ),
    (
        'worker.leaderboard_publish_max_attempts',
        '3',
        '3',
        'int',
        'worker',
        FALSE,
        'Maximum Redis PUBLISH attempts per leaderboard.updated signal before the broadcaster logs a warning and gives up. Missed signals are recovered within the leaderboard cache TTL. Requires worker restart.'
    ),
    (
        'worker.leaderboard_publish_base_delay_ms',
        '50',
        '50',
        'int',
        'worker',
        FALSE,
        'Initial exponential-backoff delay in milliseconds between leaderboard PUBLISH retries. Doubles on each subsequent attempt; total worst-case wait = base + 2×base. Requires worker restart.'
    )
ON CONFLICT (key) DO NOTHING;
