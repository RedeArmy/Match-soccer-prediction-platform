-- NOTE: 000064 and 000065 must be rolled back together (migrate down 2).
-- Rolling back 000064 alone leaves scoring_rules with extra_time_bonus /
-- penalties_bonus columns that reference semantics no longer tracked on
-- matches or predictions, creating a permanently inconsistent intermediate
-- state. Always pair: migrate -steps -2.
ALTER TABLE predictions DROP COLUMN IF EXISTS predicted_win_method;
ALTER TABLE matches     DROP COLUMN IF EXISTS win_method;
