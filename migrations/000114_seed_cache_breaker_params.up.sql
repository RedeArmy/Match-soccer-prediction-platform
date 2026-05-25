-- Migration 000114: seed Redis cache circuit-breaker parameters
--
-- Two params that control the circuit breaker wrapping the Redis cache in
-- server_compose.go::buildHandlers. While the breaker is open, all cache
-- operations (Get/Set/Delete/FlushByPrefix) are bypassed silently so the
-- service layer reads directly from PostgreSQL. This prevents a Redis outage
-- from returning errors to end users.
--
--   breaker.cache_max_fails
--     Consecutive cache errors required to trip the breaker.
--     is_runtime=FALSE: API server restart required (breaker is constructed
--     once at startup).
--
--   breaker.cache_cooldown_sec
--     Seconds the breaker stays open before allowing a single trial request.
--     is_runtime=FALSE: API server restart required.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'breaker.cache_max_fails',
        '5',
        '5',
        'int',
        'breaker',
        FALSE,
        'Consecutive Redis cache errors before the cache circuit breaker opens. While open, cache operations are bypassed and requests hit PostgreSQL directly. is_runtime=FALSE: restart required.'
    ),
    (
        'breaker.cache_cooldown_sec',
        '30',
        '30',
        'int',
        'breaker',
        FALSE,
        'Seconds the cache circuit breaker stays open before allowing a single trial request. is_runtime=FALSE: restart required.'
    )
ON CONFLICT (key) DO NOTHING;
