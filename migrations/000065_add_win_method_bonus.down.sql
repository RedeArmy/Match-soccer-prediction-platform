ALTER TABLE scoring_rules
    DROP COLUMN IF EXISTS extra_time_bonus,
    DROP COLUMN IF EXISTS penalties_bonus;
