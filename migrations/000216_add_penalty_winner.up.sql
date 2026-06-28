ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS penalty_winner TEXT
    CHECK (penalty_winner IN ('home', 'away'));

ALTER TABLE predictions
    ADD COLUMN IF NOT EXISTS predicted_penalty_winner TEXT
    CHECK (predicted_penalty_winner IN ('home', 'away'));
