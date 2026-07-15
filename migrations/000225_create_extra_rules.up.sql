-- extra_rules: admin-tunable point values for match "extras" (bonus
-- predictions), one row per extra type. Mirrors scoring_rules' shape but
-- without per-phase granularity — this quiniela launches with a small, fixed
-- pair of extra questions applied uniformly to every match, so a flat table
-- is proportionate; scoring_rules' phase dimension exists to reward
-- higher-stakes knockout fixtures, which does not apply here.
--
-- is_active: when FALSE the scoring service falls back to the compile-time
-- default in internal/domain, the same safety net scoring_rules uses.
CREATE TABLE extra_rules (
    id         SERIAL      PRIMARY KEY,
    extra_type TEXT        NOT NULL UNIQUE
               CHECK (extra_type IN ('first_scorer', 'halftime_result')),
    points     INTEGER     NOT NULL CHECK (points >= 0),
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed defaults: first_scorer is a 3-way guess resolved from goal-event data
-- (harder to call than a basic outcome), halftime_result is a 3-way guess
-- with roughly the same odds as the base correct_outcome tier (2 points in
-- scoring_rules for group_stage).
INSERT INTO extra_rules (extra_type, points) VALUES
    ('first_scorer',     3),
    ('halftime_result',  2);
