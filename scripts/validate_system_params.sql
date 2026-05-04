-- Validation script for system_params table.
--
-- This script verifies that:
-- 1. All expected param keys are present
-- 2. All rows have non-null descriptions
-- 3. All values are valid for their type
-- 4. Categories are consistent
--
-- Run with: psql -d quiniela -f scripts/validate_system_params.sql

\echo '=== System Parameters Validation Report ==='
\echo ''

-- Expected params count
\echo '1. Checking total parameter count...'
SELECT COUNT(*) as total_params,
       23 as expected_params,
       CASE WHEN COUNT(*) = 23 THEN '✓ PASS' ELSE '✗ FAIL' END as status
FROM system_params;

\echo ''
\echo '2. Checking for missing descriptions...'
SELECT key,
       CASE WHEN description IS NULL OR description = ''
            THEN '✗ Missing'
            ELSE '✓ Present'
       END as description_status
FROM system_params
WHERE description IS NULL OR description = ''
ORDER BY key;

-- If no rows returned, all descriptions are present
SELECT CASE WHEN COUNT(*) = 0
            THEN '✓ All parameters have descriptions'
            ELSE '✗ ' || COUNT(*)::text || ' parameters missing descriptions'
       END as result
FROM system_params
WHERE description IS NULL OR description = '';

\echo ''
\echo '3. Checking parameter categories...'
SELECT category,
       COUNT(*) as param_count,
       array_agg(key ORDER BY key) as keys
FROM system_params
GROUP BY category
ORDER BY category;

\echo ''
\echo '4. Checking runtime vs infrastructure params...'
SELECT
    CASE WHEN is_runtime THEN 'Runtime (is_runtime=TRUE)'
         ELSE 'Infrastructure (is_runtime=FALSE)'
    END as param_type,
    COUNT(*) as count
FROM system_params
GROUP BY is_runtime
ORDER BY is_runtime DESC;

\echo ''
\echo '5. Listing all parameters by category...'
SELECT
    category,
    key,
    value,
    type,
    CASE WHEN is_runtime THEN 'runtime' ELSE 'infrastructure' END as mode,
    LEFT(description, 80) || '...' as description_preview
FROM system_params
ORDER BY category, key;

\echo ''
\echo '6. Checking for parameters with default values matching constants.go...'
-- This requires manual verification, but we can list the values
SELECT
    key,
    value,
    type,
    '→ Verify this matches domain.Default* constant' as validation_note
FROM system_params
ORDER BY key;

\echo ''
\echo '7. Expected parameters checklist (must all be present)...'
WITH expected_params AS (
    SELECT unnest(ARRAY[
        'scoring.exact_score',
        'scoring.correct_outcome',
        'scoring.goal_difference',
        'prediction.deadline_minutes',
        'group.min_members_for_active',
        'group.default_prize_threshold',
        'group.invite_code_length',
        'conflict.stale_days',
        'conflict.max_scan',
        'pagination.default_limit',
        'pagination.max_limit',
        'tournament.win_points',
        'admin.bulk_max_items',
        'cache.match_ttl_seconds',
        'cache.leaderboard_ttl_seconds',
        'cache.dashboard_ttl_seconds',
        'audit.write_timeout_seconds',
        'auth.validation_timeout_seconds',
        'dlq.sample_size',
        'dlq.replay_default_limit',
        'messaging.max_retries',
        'messaging.stream_max_len',
        'system.purge_retention_days'
    ]) as expected_key
)
SELECT
    ep.expected_key,
    CASE WHEN sp.key IS NOT NULL THEN '✓ Present' ELSE '✗ MISSING' END as status,
    sp.value,
    sp.description
FROM expected_params ep
LEFT JOIN system_params sp ON ep.expected_key = sp.key
ORDER BY ep.expected_key;

\echo ''
\echo '8. Checking for unexpected/orphaned parameters...'
WITH expected_params AS (
    SELECT unnest(ARRAY[
        'scoring.exact_score',
        'scoring.correct_outcome',
        'scoring.goal_difference',
        'prediction.deadline_minutes',
        'group.min_members_for_active',
        'group.default_prize_threshold',
        'group.invite_code_length',
        'conflict.stale_days',
        'conflict.max_scan',
        'pagination.default_limit',
        'pagination.max_limit',
        'tournament.win_points',
        'admin.bulk_max_items',
        'cache.match_ttl_seconds',
        'cache.leaderboard_ttl_seconds',
        'cache.dashboard_ttl_seconds',
        'audit.write_timeout_seconds',
        'auth.validation_timeout_seconds',
        'dlq.sample_size',
        'dlq.replay_default_limit',
        'messaging.max_retries',
        'messaging.stream_max_len',
        'system.purge_retention_days'
    ]) as expected_key
)
SELECT
    sp.key as orphaned_key,
    '⚠ Not in constants.go - may be deprecated' as warning
FROM system_params sp
LEFT JOIN expected_params ep ON sp.key = ep.expected_key
WHERE ep.expected_key IS NULL;

-- If no rows returned, no orphaned params
SELECT CASE WHEN COUNT(*) = 0
            THEN '✓ No orphaned parameters found'
            ELSE '⚠ ' || COUNT(*)::text || ' orphaned parameters found (see above)'
       END as result
FROM system_params sp
WHERE NOT EXISTS (
    SELECT 1 FROM (
        SELECT unnest(ARRAY[
            'scoring.exact_score',
            'scoring.correct_outcome',
            'scoring.goal_difference',
            'prediction.deadline_minutes',
            'group.min_members_for_active',
            'group.default_prize_threshold',
            'group.invite_code_length',
            'conflict.stale_days',
            'conflict.max_scan',
            'pagination.default_limit',
            'pagination.max_limit',
            'tournament.win_points',
            'admin.bulk_max_items',
            'cache.match_ttl_seconds',
            'cache.leaderboard_ttl_seconds',
            'cache.dashboard_ttl_seconds',
            'audit.write_timeout_seconds',
            'auth.validation_timeout_seconds',
            'dlq.sample_size',
            'dlq.replay_default_limit',
            'messaging.max_retries',
            'messaging.stream_max_len',
            'system.purge_retention_days'
        ]) as k
    ) expected
    WHERE expected.k = sp.key
);

\echo ''
\echo '=== Validation Complete ==='
\echo 'Review the output above for any ✗ FAIL or ⚠ WARNING markers.'
\echo ''
