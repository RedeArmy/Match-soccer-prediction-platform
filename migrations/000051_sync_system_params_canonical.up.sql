-- Migration: sync system_params to canonical post-refactoring state
--
-- This migration is the authoritative snapshot of every system_params row
-- after the group-size refactoring (migration 050). It:
--
--   1. Upserts all 22 ParamKey* domain constants with the correct default
--      value, type, category, is_runtime flag, and description.
--   2. Ensures group.min_members_for_active = '5' (reflects MinMembersPerGroup).
--   3. Does NOT include group.default_prize_threshold (retired by migration 050).
--
-- ON CONFLICT always updates value, type, category, is_runtime, and description
-- so that a DB that missed an earlier migration (e.g. a stale staging schema)
-- is brought back into sync. The value field IS updated here because this
-- migration encodes the canonical defaults; operator overrides must be
-- re-applied manually after running this migration.

INSERT INTO system_params (key, value, type, category, is_runtime, description) VALUES

    -- Scoring (runtime: re-read on every ScoreMatch call)
    ('scoring.exact_score',      '5', 'int', 'scoring', TRUE,
     'Points awarded when prediction exactly matches the final score (e.g. 2-1 → 2-1)'),

    ('scoring.correct_outcome',  '2', 'int', 'scoring', TRUE,
     'Points awarded for a correct win/draw/loss outcome when the exact score is wrong'),

    ('scoring.goal_difference',  '1', 'int', 'scoring', TRUE,
     'Bonus point when predicted goal margin equals actual margin; does not apply to draws'),

    -- Prediction (runtime: re-read on every Submit/Update call)
    ('prediction.deadline_minutes', '5', 'int', 'prediction', TRUE,
     'Minutes before kick-off after which predictions are locked; submissions after (kickoff - deadline) are rejected'),

    -- Group lifecycle (runtime)
    ('group.min_members_for_active', '5', 'int', 'group', TRUE,
     'Minimum active paid members for prize eligibility and payment processing; groups below this remain inactive'),

    ('group.invite_code_length', '10', 'int', 'group', TRUE,
     'Number of characters in a randomly generated group invite code'),

    -- Conflict detection (runtime: tunable without restart)
    ('conflict.stale_days', '7', 'int', 'conflict', TRUE,
     'Age in days after which pending payments or memberships are flagged as stale conflicts'),

    ('conflict.max_scan', '5000', 'int', 'conflict', TRUE,
     'Maximum conflicts loaded into memory by ConflictSummary; prevents OOM when the conflict backlog is pathologically large'),

    -- Pagination (runtime: re-read per paginated request)
    ('pagination.default_limit',  '50',  'int', 'pagination', TRUE,
     'Default page size when the client omits the ?limit parameter'),

    ('pagination.max_limit',     '200', 'int', 'pagination', TRUE,
     'Maximum allowed page size; requests with ?limit > max_limit are capped to this value'),

    -- Tournament standings (runtime)
    ('tournament.win_points', '3', 'int', 'tournament', TRUE,
     'Standing points awarded for a group-stage win (FIFA 3-point rule)'),

    -- Admin bulk operations (runtime: tunable during high-load without restart)
    ('admin.bulk_max_items', '1000', 'int', 'admin', TRUE,
     'Maximum IDs accepted in a single bulk admin operation; prevents oversized ANY($1) queries'),

    -- Cache TTLs
    ('cache.match_ttl_seconds',       '300', 'int', 'cache', FALSE,
     'Match-list cache TTL in seconds; restart required to apply changes'),

    ('cache.leaderboard_ttl_seconds',  '60', 'int', 'cache', TRUE,
     'Leaderboard cache TTL in seconds; applied immediately via mutation hook without restart'),

    ('cache.dashboard_ttl_seconds',    '30', 'int', 'cache', TRUE,
     'Dashboard stats cache TTL in seconds; set to 0 to disable the cache'),

    -- Infrastructure timeouts (restart to apply)
    ('audit.write_timeout_seconds',    '5', 'int', 'system', FALSE,
     'Maximum seconds the audit log goroutine waits to persist an entry before giving up'),

    ('auth.validation_timeout_seconds', '5', 'int', 'system', FALSE,
     'JWKS warm-up timeout in seconds during auth middleware initialization at process startup'),

    -- Dead-letter queue (restart to apply)
    ('dlq.sample_size',          '5',  'int', 'dlq', FALSE,
     'Maximum number of dead-letter queue entries returned in the Stats sample field'),

    ('dlq.replay_default_limit', '10', 'int', 'dlq', FALSE,
     'Default number of DLQ entries replayed when the caller does not supply an explicit limit'),

    -- Messaging / Redis Streams (restart to apply)
    ('messaging.max_retries',    '3',      'int', 'messaging', FALSE,
     'Total handler attempts before an event is dead-lettered; includes the initial attempt'),

    ('messaging.stream_max_len', '600000', 'int', 'messaging', FALSE,
     'Redis Stream MAXLEN cap (approximate); at ~1 event/sec this retains roughly 7 days of history'),

    -- System lifecycle (restart to apply)
    ('system.purge_retention_days', '30', 'int', 'system', FALSE,
     'Age in days after which soft-deleted users and quinielas are permanently removed by the worker purge goroutine')

ON CONFLICT (key) DO UPDATE SET
    value       = EXCLUDED.value,
    type        = EXCLUDED.type,
    category    = EXCLUDED.category,
    is_runtime  = EXCLUDED.is_runtime,
    description = EXCLUDED.description,
    updated_at  = NOW();
