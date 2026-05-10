ALTER TABLE scoring_rules
    ADD COLUMN extra_time_bonus INTEGER NOT NULL DEFAULT 0 CHECK (extra_time_bonus >= 0),
    ADD COLUMN penalties_bonus  INTEGER NOT NULL DEFAULT 0 CHECK (penalties_bonus  >= 0);

-- Seed the standard bonus values for all knockout phases.
-- Group-stage row retains 0/0 (no knockout bonuses).
UPDATE scoring_rules SET extra_time_bonus = 1, penalties_bonus = 2
 WHERE phase IN ('round_of_32', 'round_of_16', 'quarter_final', 'semi_final', 'third_place', 'final');
