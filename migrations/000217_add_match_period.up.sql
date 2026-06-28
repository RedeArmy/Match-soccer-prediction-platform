ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS period             TEXT,
    ADD COLUMN IF NOT EXISTS penalty_home_score INT,
    ADD COLUMN IF NOT EXISTS penalty_away_score INT;
