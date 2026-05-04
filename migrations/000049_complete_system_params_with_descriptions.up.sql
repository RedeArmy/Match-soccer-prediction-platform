-- Complete and validate all system_params rows with descriptions.
--
-- This migration ensures every ParamKey* constant defined in domain/constants.go
-- is present in system_params with a complete description field. Previous
-- migrations (000040-000048) created the rows, but some lack descriptions.
--
-- This is the canonical "source of truth" migration: it lists every system
-- parameter with its full metadata. ON CONFLICT (key) DO UPDATE ensures that
-- missing descriptions are backfilled while preserving operator overrides to
-- the value field.
--
-- Categories and their params:
--   scoring    — Points awarded per prediction outcome
--   prediction — Submission deadline rules
--   group      — Quiniela / group lifecycle rules
--   conflict   — Operational conflict-detection thresholds
--   pagination — API page-size limits
--   tournament — Standings and scoring rules
--   admin      — Bulk operation limits
--   cache      — Redis TTL configuration
--   system     — Infrastructure timeouts and lifecycle
--   dlq        — Dead-letter queue behaviour
--   messaging  — Event-bus retry policy
--   auth       — Authentication configuration

INSERT INTO system_params (key, value, type, category, is_runtime, description) VALUES
    -- Scoring params (runtime: re-read on every ScoreMatch call)
    ('scoring.exact_score', '5', 'int', 'scoring', TRUE,
     'Points awarded when prediction exactly matches the final score'),

    ('scoring.correct_outcome', '2', 'int', 'scoring', TRUE,
     'Points awarded when prediction has correct outcome (win/loss/draw) but wrong score'),

    ('scoring.goal_difference', '1', 'int', 'scoring', TRUE,
     'Bonus point awarded when predicted goal margin equals actual margin (does not apply to draws)'),

    -- Prediction params (runtime: re-read on every Submit/Update call)
    ('prediction.deadline_minutes', '5', 'int', 'prediction', TRUE,
     'Minutes before kick-off when predictions are locked. Submissions after (kickoff - deadline) are rejected'),

    -- Group / quiniela lifecycle params (runtime)
    ('group.min_members_for_active', '3', 'int', 'group', TRUE,
     'Minimum active members required for a quiniela to be eligible for payment processing and prizes'),

    ('group.default_prize_threshold', '3', 'int', 'group', TRUE,
     'Default prize distribution ratio. Winner count = max(1, floor(members / threshold))'),

    ('group.invite_code_length', '10', 'int', 'group', TRUE,
     'Length of generated invite codes for joining quinielas'),

    -- Conflict detection params (runtime)
    ('conflict.stale_days', '7', 'int', 'conflict', TRUE,
     'Age in days after which pending payments and memberships are flagged as stale conflicts'),

    ('conflict.max_scan', '5000', 'int', 'conflict', TRUE,
     'Maximum conflicts loaded into memory by ConflictSummary. Prevents OOM when backlog is pathologically large'),

    -- Pagination params (runtime: safe to read on every paginated request)
    ('pagination.default_limit', '50', 'int', 'pagination', TRUE,
     'Default page size when client does not specify ?limit parameter'),

    ('pagination.max_limit', '200', 'int', 'pagination', TRUE,
     'Maximum allowed page size. Requests with ?limit > max_limit are capped to this value'),

    -- Tournament standings params (runtime)
    ('tournament.win_points', '3', 'int', 'tournament', TRUE,
     'Standing points awarded for a group-stage win (FIFA 3-point rule)'),

    -- Admin bulk operation params (runtime: can be tuned during high-load without restart)
    ('admin.bulk_max_items', '1000', 'int', 'admin', TRUE,
     'Maximum IDs accepted in bulk operations (BulkDeleteGroups, BulkRemoveMembers). Prevents oversized ANY($1) queries'),

    -- Cache TTL params (infrastructure: most require restart; dashboard is runtime-tunable)
    ('cache.match_ttl_seconds', '300', 'int', 'cache', FALSE,
     'Match-list cache TTL in seconds. Restart required to apply changes'),

    ('cache.leaderboard_ttl_seconds', '60', 'int', 'cache', FALSE,
     'Leaderboard cache TTL in seconds. Restart required to apply changes'),

    ('cache.dashboard_ttl_seconds', '30', 'int', 'cache', TRUE,
     'Dashboard stats cache TTL in seconds. Runtime-tunable; set to 0 to disable caching'),

    -- Infrastructure timeout params (restart to apply)
    ('audit.write_timeout_seconds', '5', 'int', 'system', FALSE,
     'Maximum time audit log goroutine waits to persist an entry before giving up'),

    ('auth.validation_timeout_seconds', '5', 'int', 'system', FALSE,
     'JWKS warm-up timeout during auth middleware initialization'),

    -- Dead-letter queue params (infrastructure; restart to apply)
    ('dlq.sample_size', '5', 'int', 'dlq', FALSE,
     'Maximum number of DLQ entries returned in Stats sample field'),

    ('dlq.replay_default_limit', '10', 'int', 'dlq', FALSE,
     'Default number of DLQ entries replayed when caller does not supply explicit limit'),

    -- Messaging / event-bus params (infrastructure; restart to apply)
    ('messaging.max_retries', '3', 'int', 'messaging', FALSE,
     'Total handler attempts before an event is dead-lettered (includes initial attempt)'),

    ('messaging.stream_max_len', '600000', 'int', 'messaging', FALSE,
     'Redis Stream MAXLEN cap. At ~1 event/sec this retains roughly 7 days of history'),

    -- System lifecycle params (infrastructure; restart to apply)
    ('system.purge_retention_days', '30', 'int', 'system', FALSE,
     'Age in days after which soft-deleted users and quinielas are permanently removed by worker purge goroutine')

ON CONFLICT (key) DO UPDATE SET
    description = EXCLUDED.description,
    type = EXCLUDED.type,
    category = EXCLUDED.category,
    is_runtime = EXCLUDED.is_runtime
    -- Note: value is intentionally NOT updated to preserve operator overrides
WHERE system_params.description IS NULL
   OR system_params.description = '';
