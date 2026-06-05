ALTER TABLE matches DROP CONSTRAINT IF EXISTS uq_matches_match_code;
ALTER TABLE matches DROP COLUMN  IF EXISTS match_code;
