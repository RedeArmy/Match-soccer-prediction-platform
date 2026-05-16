-- Migration: canonicalize is_runtime flags for all 42 system parameters
--
-- Earlier seed migrations set some is_runtime flags inconsistently. For example,
-- migration 000049 seeded cache.leaderboard_ttl_seconds with is_runtime=FALSE
-- before migration 000051 corrected it to TRUE. This migration establishes the
-- single authoritative source of truth for every flag value and closes the gap
-- that validate-params now checks.
--
-- is_runtime = TRUE  → the service uses a 30 s cache TTL. Operator changes
--                       propagate within one cache window without a process restart.
--                       These params are safe to tune during a live tournament.
--
-- is_runtime = FALSE → the service uses a 5 min cache TTL, and most consumers
--                       read the value once at process startup. A restart is
--                       required to guarantee the new value takes effect.
--
-- This migration only touches the is_runtime column. The value, default_value,
-- type, category, and description columns are intentionally left unchanged so
-- operator overrides are preserved.
--
-- Idempotent: safe to re-run; UPDATE WHERE is a no-op when the flag already
-- has the correct value.

-- ── Runtime params (is_runtime = TRUE) ────────────────────────────────────────
-- These are read on every applicable request or propagate via mutation hooks.

UPDATE system_params
SET    is_runtime = TRUE,
       updated_at = NOW()
WHERE  key IN (
    -- Scoring (re-read on every ScoreMatch call)
    'scoring.exact_score',
    'scoring.correct_outcome',
    'scoring.goal_difference',
    'scoring.extra_time_bonus',
    'scoring.penalties_bonus',
    -- Prediction (re-read on every prediction submit/update)
    'prediction.deadline_minutes',
    -- Group lifecycle (enforced at request time)
    'group.min_members_for_active',
    'group.max_size',
    'group.invite_code_length',
    -- Conflict detection (read per ConflictSummary/ListConflicts call)
    'conflict.stale_days',
    'conflict.max_scan',
    -- Pagination (read on every paginated request)
    'pagination.default_limit',
    'pagination.max_limit',
    -- Tournament standings (read on every group-stage scoring event)
    'tournament.win_points',
    -- Admin bulk operations (read per bulk-action request)
    'admin.bulk_max_items',
    -- Cache TTLs with active mutation hooks
    'cache.leaderboard_ttl_seconds',  -- CachedRankingService.UpdateTTL hook
    'cache.dashboard_ttl_seconds',    -- CachedAdminReadService hook
    -- Payment / balance (read per payment/withdrawal request)
    'payment.max_upload_bytes',
    'payment.withdrawal_min_cents',
    'payment.withdrawal_max_cents',
    'payment.bank_transfer_min_amount_cents',
    'payment.bank_transfer_max_amount_cents',
    'payment.intent_ttl_minutes'
)
AND is_runtime IS DISTINCT FROM TRUE;

-- ── Non-runtime params (is_runtime = FALSE) ───────────────────────────────────
-- These are read once at process startup or require a restart to guarantee
-- the new value takes effect.

UPDATE system_params
SET    is_runtime = FALSE,
       updated_at = NOW()
WHERE  key IN (
    -- Cache TTL without a mutation hook (restart required)
    'cache.match_ttl_seconds',
    -- Infrastructure timeouts
    'audit.write_timeout_seconds',
    'audit.max_retries',
    'audit.retry_delay_ms',
    'auth.validation_timeout_seconds',
    -- Dead-letter queue configuration
    'dlq.sample_size',
    'dlq.replay_default_limit',
    -- Messaging / Redis Streams consumer pool
    'messaging.max_retries',
    'messaging.stream_max_len',
    'messaging.stream_worker_count',
    'messaging.stream_read_block_sec',
    -- Worker: snapshot generation
    'worker.snapshot_concurrency',
    'worker.snapshot_retry_base_ms',
    'worker.snapshot_max_attempts',
    -- Worker: background maintenance
    'worker.dlq_monitor_interval_sec',
    'worker.purge_interval_hours',
    -- System lifecycle
    'system.purge_retention_days',
    -- API request limits
    'api.body_size_limit_bytes',
    -- Snapshot retention
    'snapshot.keep_latest_count'
)
AND is_runtime IS DISTINCT FROM FALSE;
