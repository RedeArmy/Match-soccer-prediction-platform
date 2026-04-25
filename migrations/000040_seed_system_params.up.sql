-- Seed system_params with all runtime-configurable values.
--
-- Each row documents one tuneable constant and sets its initial value to match
-- the hard-coded default currently in the source. Operators can change values
-- at runtime via PATCH /admin/system-params/{key} without a code deploy
-- (business params only; infrastructure params require a process restart).
--
-- Categories:
--   scoring    — points awarded per prediction outcome
--   prediction — submission deadline rules
--   conflict   — operational conflict-detection thresholds
--   group      — quiniela / group lifecycle rules
--   pagination — API page-size limits (safe to read on every request)
--   tournament — standings and scoring rules
--   system     — process-level infrastructure (restart to apply)
--   cache      — Redis TTLs (infrastructure; restart to apply)
--   dlq        — dead-letter queue behaviour (infrastructure)
--   messaging  — event-bus retry policy (infrastructure)
--
-- ON CONFLICT DO NOTHING ensures idempotency: re-running the migration or
-- applying it against an instance that already has manual overrides is safe.

INSERT INTO system_params (key, value, type, category, is_runtime) VALUES
    -- Scoring params — read on every ScoreMatch call; safe for dynamic reads
    ('scoring.exact_score',     '5', 'int', 'scoring', TRUE),
    ('scoring.correct_outcome', '2', 'int', 'scoring', TRUE),
    ('scoring.goal_difference', '1', 'int', 'scoring', TRUE),

    -- Prediction params — read on every Submit/Update call
    ('prediction.deadline_minutes', '5', 'int', 'prediction', TRUE),

    -- Group / quiniela lifecycle params
    ('group.min_members_for_active',  '3',  'int', 'group', TRUE),
    ('group.default_prize_threshold', '3',  'int', 'group', TRUE),
    ('group.invite_code_length',      '10', 'int', 'group', TRUE),

    -- Conflict detection thresholds
    ('conflict.stale_days', '7', 'int', 'conflict', TRUE),

    -- Pagination limits — safe to read dynamically on every request
    ('pagination.default_limit', '50',  'int', 'pagination', TRUE),
    ('pagination.max_limit',     '200', 'int', 'pagination', TRUE),

    -- Tournament standings — read dynamically by TournamentService
    ('tournament.win_points', '3', 'int', 'tournament', TRUE),

    -- System-level infrastructure — read once at startup; restart to apply
    ('audit.write_timeout_seconds',  '5', 'int', 'system', FALSE),
    ('auth.validation_timeout_seconds', '5', 'int', 'system', FALSE),

    -- Cache TTLs — read once at startup; restart to apply
    ('cache.match_ttl_seconds',       '300', 'int', 'cache', FALSE),
    ('cache.leaderboard_ttl_seconds', '60',  'int', 'cache', FALSE),

    -- DLQ behaviour — read once at startup; restart to apply
    ('dlq.sample_size',            '5',  'int', 'messaging', FALSE),
    ('dlq.replay_default_limit',   '10', 'int', 'messaging', FALSE),

    -- Messaging / event-bus retry policy — read once at startup; restart to apply
    ('messaging.max_retries',    '3',      'int', 'messaging', FALSE),
    ('messaging.stream_max_len', '600000', 'int', 'messaging', FALSE)
ON CONFLICT (key) DO NOTHING;
