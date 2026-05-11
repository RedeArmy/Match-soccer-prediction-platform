-- NOTE: 000065 and 000064 must be rolled back together (migrate down 2).
-- Rolling back 000065 alone leaves matches.win_method and
-- predictions.predicted_win_method in place while removing the bonus columns
-- that give those fields their scoring meaning, creating a permanently
-- inconsistent intermediate state. Always pair: migrate -steps -2.
ALTER TABLE scoring_rules
    DROP COLUMN IF EXISTS extra_time_bonus,
    DROP COLUMN IF EXISTS penalties_bonus;
