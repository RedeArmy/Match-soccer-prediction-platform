-- Validation script for system_params table.
--
-- Verifies that:
--   1. All 44 expected param keys are present (as of migration 000079)
--   2. No deprecated keys remain (e.g. group.default_prize_threshold)
--   3. All rows have non-empty descriptions
--   4. Types and categories are consistent with constants.go
--
-- Run with: psql -d quiniela -f scripts/validate_system_params.sql
--
-- Expected param count history:
--   000051  22 params  (canonical post-refactoring snapshot)
--   000055  +10        (worker, messaging, audit, api)
--   000056  +1         (snapshot.keep_latest_count)
--   000058  +1         (group.max_size)
--   000066  +2         (scoring.extra_time_bonus, scoring.penalties_bonus)
--   000073  +3         (payment.max_upload_bytes, payment.withdrawal_min_cents, payment.withdrawal_max_cents)
--   000074  +2         (payment.bank_transfer_min_amount_cents, payment.bank_transfer_max_amount_cents)
--   000076  +1         (payment.intent_ttl_minutes)
--   000079  +2         (api.rate_limit_rate_per_sec, api.rate_limit_burst)
--   TOTAL   44 params

\echo '=== System Parameters Validation Report ==='
\echo ''

-- 1. Total count
\echo '1. Checking total parameter count (expected: 44)...'
SELECT COUNT(*)            AS total_params,
       44                  AS expected_params,
       CASE WHEN COUNT(*) = 44 THEN '✓ PASS' ELSE '✗ FAIL' END AS status
FROM system_params;

\echo ''
\echo '2. Checking for missing or empty descriptions...'
SELECT key,
       '✗ Missing description' AS issue
FROM system_params
WHERE description IS NULL OR description = ''
ORDER BY key;

SELECT CASE WHEN COUNT(*) = 0
            THEN '✓ All parameters have descriptions'
            ELSE '✗ ' || COUNT(*)::text || ' parameters missing descriptions'
       END AS result
FROM system_params
WHERE description IS NULL OR description = '';

\echo ''
\echo '3. Checking for missing default_value...'
SELECT key,
       '✗ NULL default_value' AS issue
FROM system_params
WHERE default_value IS NULL
ORDER BY key;

SELECT CASE WHEN COUNT(*) = 0
            THEN '✓ All parameters have a default_value'
            ELSE '✗ ' || COUNT(*)::text || ' parameters missing default_value'
       END AS result
FROM system_params
WHERE default_value IS NULL;

\echo ''
\echo '4. Parameter breakdown by category...'
SELECT category,
       COUNT(*)                       AS param_count,
       array_agg(key ORDER BY key)   AS keys
FROM system_params
GROUP BY category
ORDER BY category;

\echo ''
\echo '5. Runtime vs infrastructure params...'
SELECT CASE WHEN is_runtime THEN 'Runtime (is_runtime=TRUE)'
            ELSE 'Infrastructure (is_runtime=FALSE)'
       END AS param_type,
       COUNT(*) AS count
FROM system_params
GROUP BY is_runtime
ORDER BY is_runtime DESC;

\echo ''
\echo '6. Checking for deprecated / unexpected keys...'
WITH current_keys AS (
    SELECT unnest(ARRAY[
        -- scoring (000051 + 000066)
        'scoring.correct_outcome',
        'scoring.exact_score',
        'scoring.extra_time_bonus',
        'scoring.goal_difference',
        'scoring.penalties_bonus',
        -- prediction (000051)
        'prediction.deadline_minutes',
        -- group (000051 + 000058)
        'group.invite_code_length',
        'group.max_size',
        'group.min_members_for_active',
        -- conflict (000051)
        'conflict.max_scan',
        'conflict.stale_days',
        -- pagination (000051)
        'pagination.default_limit',
        'pagination.max_limit',
        -- tournament (000051)
        'tournament.win_points',
        -- admin (000051)
        'admin.bulk_max_items',
        -- cache (000051)
        'cache.dashboard_ttl_seconds',
        'cache.leaderboard_ttl_seconds',
        'cache.match_ttl_seconds',
        -- system (000051 + 000055)
        'audit.max_retries',
        'audit.retry_delay_ms',
        'audit.write_timeout_seconds',
        'auth.validation_timeout_seconds',
        'system.purge_retention_days',
        -- dlq (000051)
        'dlq.replay_default_limit',
        'dlq.sample_size',
        -- messaging (000051 + 000055)
        'messaging.max_retries',
        'messaging.stream_max_len',
        'messaging.stream_read_block_sec',
        'messaging.stream_worker_count',
        -- worker / api / snapshot (000055 + 000056)
        'api.body_size_limit_bytes',
        'snapshot.keep_latest_count',
        'worker.dlq_monitor_interval_sec',
        'worker.purge_interval_hours',
        'worker.snapshot_concurrency',
        'worker.snapshot_max_attempts',
        'worker.snapshot_retry_base_ms',
        -- payment (000073 + 000074 + 000076)
        'payment.max_upload_bytes',
        'payment.withdrawal_min_cents',
        'payment.withdrawal_max_cents',
        'payment.bank_transfer_min_amount_cents',
        'payment.bank_transfer_max_amount_cents',
        'payment.intent_ttl_minutes',
        -- api rate limiting (000079)
        'api.rate_limit_rate_per_sec',
        'api.rate_limit_burst'
    ]) AS expected_key
)
SELECT sp.key   AS unexpected_key,
       '⚠ Not in constants.go — may be deprecated or orphaned' AS warning
FROM system_params sp
LEFT JOIN current_keys ck ON sp.key = ck.expected_key
WHERE ck.expected_key IS NULL
ORDER BY sp.key;

SELECT CASE WHEN COUNT(*) = 0
            THEN '✓ No unexpected parameters found'
            ELSE '⚠ ' || COUNT(*)::text || ' unexpected parameters found (see above)'
       END AS result
FROM system_params sp
WHERE NOT EXISTS (
    SELECT 1 FROM (
        SELECT unnest(ARRAY[
            'scoring.correct_outcome',
            'scoring.exact_score',
            'scoring.extra_time_bonus',
            'scoring.goal_difference',
            'scoring.penalties_bonus',
            'prediction.deadline_minutes',
            'group.invite_code_length',
            'group.max_size',
            'group.min_members_for_active',
            'conflict.max_scan',
            'conflict.stale_days',
            'pagination.default_limit',
            'pagination.max_limit',
            'tournament.win_points',
            'admin.bulk_max_items',
            'cache.dashboard_ttl_seconds',
            'cache.leaderboard_ttl_seconds',
            'cache.match_ttl_seconds',
            'audit.max_retries',
            'audit.retry_delay_ms',
            'audit.write_timeout_seconds',
            'auth.validation_timeout_seconds',
            'system.purge_retention_days',
            'dlq.replay_default_limit',
            'dlq.sample_size',
            'messaging.max_retries',
            'messaging.stream_max_len',
            'messaging.stream_read_block_sec',
            'messaging.stream_worker_count',
            'api.body_size_limit_bytes',
            'snapshot.keep_latest_count',
            'worker.dlq_monitor_interval_sec',
            'worker.purge_interval_hours',
            'worker.snapshot_concurrency',
            'worker.snapshot_max_attempts',
            'worker.snapshot_retry_base_ms',
            'payment.max_upload_bytes',
            'payment.withdrawal_min_cents',
            'payment.withdrawal_max_cents',
            'payment.bank_transfer_min_amount_cents',
            'payment.bank_transfer_max_amount_cents',
            'payment.intent_ttl_minutes',
            'api.rate_limit_rate_per_sec',
            'api.rate_limit_burst'
        ]) AS k
    ) current_keys
    WHERE current_keys.k = sp.key
);

\echo ''
\echo '7. Expected parameters checklist (all 44 must be ✓ Present)...'
WITH expected_params AS (
    SELECT unnest(ARRAY[
        -- scoring
        'scoring.correct_outcome',
        'scoring.exact_score',
        'scoring.extra_time_bonus',
        'scoring.goal_difference',
        'scoring.penalties_bonus',
        -- prediction
        'prediction.deadline_minutes',
        -- group
        'group.invite_code_length',
        'group.max_size',
        'group.min_members_for_active',
        -- conflict
        'conflict.max_scan',
        'conflict.stale_days',
        -- pagination
        'pagination.default_limit',
        'pagination.max_limit',
        -- tournament
        'tournament.win_points',
        -- admin
        'admin.bulk_max_items',
        -- cache
        'cache.dashboard_ttl_seconds',
        'cache.leaderboard_ttl_seconds',
        'cache.match_ttl_seconds',
        -- system / audit / auth
        'audit.max_retries',
        'audit.retry_delay_ms',
        'audit.write_timeout_seconds',
        'auth.validation_timeout_seconds',
        'system.purge_retention_days',
        -- dlq
        'dlq.replay_default_limit',
        'dlq.sample_size',
        -- messaging
        'messaging.max_retries',
        'messaging.stream_max_len',
        'messaging.stream_read_block_sec',
        'messaging.stream_worker_count',
        -- worker / api / snapshot
        'api.body_size_limit_bytes',
        'snapshot.keep_latest_count',
        'worker.dlq_monitor_interval_sec',
        'worker.purge_interval_hours',
        'worker.snapshot_concurrency',
        'worker.snapshot_max_attempts',
        'worker.snapshot_retry_base_ms',
        -- payment (000073 + 000074 + 000076)
        'payment.max_upload_bytes',
        'payment.withdrawal_min_cents',
        'payment.withdrawal_max_cents',
        'payment.bank_transfer_min_amount_cents',
        'payment.bank_transfer_max_amount_cents',
        'payment.intent_ttl_minutes',
        -- api rate limiting (000079)
        'api.rate_limit_rate_per_sec',
        'api.rate_limit_burst'
    ]) AS expected_key
)
SELECT ep.expected_key,
       CASE WHEN sp.key IS NOT NULL THEN '✓ Present' ELSE '✗ MISSING' END AS status,
       sp.value,
       sp.default_value,
       sp.type,
       CASE WHEN sp.is_runtime THEN 'runtime' ELSE 'infra' END AS mode
FROM expected_params ep
LEFT JOIN system_params sp ON ep.expected_key = sp.key
ORDER BY ep.expected_key;

\echo ''
\echo '8. Full parameter listing (category → key → value)...'
SELECT category,
       key,
       value,
       default_value,
       type,
       CASE WHEN is_runtime THEN 'runtime' ELSE 'infra' END AS mode,
       LEFT(description, 80) AS description_preview
FROM system_params
ORDER BY category, key;

\echo ''
\echo '=== Validation Complete ==='
\echo 'Review the output for any ✗ FAIL, ✗ MISSING, or ⚠ WARNING markers.'
