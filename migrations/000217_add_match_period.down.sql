ALTER TABLE matches
    DROP COLUMN IF EXISTS period,
    DROP COLUMN IF EXISTS penalty_home_score,
    DROP COLUMN IF EXISTS penalty_away_score;
