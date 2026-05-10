-- Scoring rules table: one row per tournament phase, configurable by operators
-- without code changes or redeployment. Knockout rounds carry progressively
-- higher point values than the group stage to reward correct predictions on
-- high-stakes fixtures.
--
-- is_active: when FALSE the scoring service falls back to the global
-- system_params values (scoring.exact_score, scoring.correct_outcome,
-- scoring.goal_difference) so a misconfigured rule can be rolled back
-- immediately by toggling the flag rather than changing point values.

CREATE TABLE scoring_rules (
    id              SERIAL      PRIMARY KEY,
    phase           TEXT        NOT NULL UNIQUE
                    CHECK (phase IN (
                        'group_stage', 'round_of_32', 'round_of_16',
                        'quarter_final', 'semi_final', 'third_place', 'final'
                    )),
    exact_score     INTEGER     NOT NULL CHECK (exact_score     >= 0),
    correct_outcome INTEGER     NOT NULL CHECK (correct_outcome >= 0),
    goal_difference INTEGER     NOT NULL CHECK (goal_difference >= 0),
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed all seven phases. Group stage preserves the historic defaults exactly
-- (5/2/1) so existing scored matches are unaffected. Knockout phases carry
-- escalating multipliers reflecting the higher stakes of elimination fixtures.
INSERT INTO scoring_rules (phase, exact_score, correct_outcome, goal_difference) VALUES
    ('group_stage',    5,  2, 1),
    ('round_of_32',    6,  3, 1),
    ('round_of_16',    8,  4, 2),
    ('quarter_final', 10,  5, 2),
    ('semi_final',    12,  6, 3),
    ('third_place',   12,  6, 3),
    ('final',         15,  8, 3);
