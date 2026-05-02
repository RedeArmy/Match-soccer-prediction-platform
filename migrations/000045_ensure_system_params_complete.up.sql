-- Ensure every system param key defined in domain/constants.go is present.
--
-- All rows were originally seeded by migrations 000040-000042; 000044 corrected
-- the dlq.* category from 'messaging' to 'dlq'. This migration provides a
-- self-healing safety net: if any row is accidentally deleted it is re-inserted
-- with its documented default value, type, and the corrected category.
--
-- ON CONFLICT DO NOTHING makes this idempotent: existing rows — including
-- operator overrides — are left untouched.

INSERT INTO system_params (key, value, type, category, is_runtime) VALUES
    -- Scoring (runtime: re-read on every ScoreMatch call)
    ('scoring.exact_score',             '5',       'int', 'scoring',    TRUE),
    ('scoring.correct_outcome',         '2',       'int', 'scoring',    TRUE),
    ('scoring.goal_difference',         '1',       'int', 'scoring',    TRUE),

    -- Prediction (runtime: re-read on every Submit/Update call)
    ('prediction.deadline_minutes',     '5',       'int', 'prediction', TRUE),

    -- Group / quiniela lifecycle (runtime)
    ('group.min_members_for_active',    '3',       'int', 'group',      TRUE),
    ('group.default_prize_threshold',   '3',       'int', 'group',      TRUE),
    ('group.invite_code_length',        '10',      'int', 'group',      TRUE),

    -- Conflict detection (runtime)
    ('conflict.stale_days',             '7',       'int', 'conflict',   TRUE),

    -- Pagination (runtime: re-read on every paginated request)
    ('pagination.default_limit',        '50',      'int', 'pagination', TRUE),
    ('pagination.max_limit',            '200',     'int', 'pagination', TRUE),

    -- Tournament standings (runtime)
    ('tournament.win_points',           '3',       'int', 'tournament', TRUE),

    -- Admin bulk operations (runtime: can be lowered during high-load without restart)
    ('admin.bulk_max_items',            '1000',    'int', 'admin',      TRUE),

    -- Cache TTLs (infrastructure: restart to apply; dashboard is runtime-tunable)
    ('cache.match_ttl_seconds',         '300',     'int', 'cache',      FALSE),
    ('cache.leaderboard_ttl_seconds',   '60',      'int', 'cache',      FALSE),
    ('cache.dashboard_ttl_seconds',     '30',      'int', 'cache',      TRUE),

    -- Infrastructure timeouts (restart to apply)
    ('audit.write_timeout_seconds',     '5',       'int', 'system',     FALSE),
    ('auth.validation_timeout_seconds', '5',       'int', 'system',     FALSE),

    -- Dead-letter queue (infrastructure; restart to apply)
    ('dlq.sample_size',                 '5',       'int', 'dlq',        FALSE),
    ('dlq.replay_default_limit',        '10',      'int', 'dlq',        FALSE),

    -- Messaging / event-bus (infrastructure; restart to apply)
    ('messaging.max_retries',           '3',       'int', 'messaging',  FALSE),
    ('messaging.stream_max_len',        '600000',  'int', 'messaging',  FALSE)

ON CONFLICT (key) DO NOTHING;
