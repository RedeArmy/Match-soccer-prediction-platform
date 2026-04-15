-- Add prize_threshold to quinielas.
--
-- prize_threshold controls the proportional prize distribution formula:
--   winner_count = max(1, floor(member_count / prize_threshold))
--
-- A value of 3 means roughly 1-in-3 active+paid members win a prize,
-- which was the product default chosen at design time. The CHECK constraint
-- prevents a division-by-zero and a nonsensical zero threshold at the
-- persistence layer, reinforcing the application-level validation.

ALTER TABLE quinielas
    ADD COLUMN prize_threshold INT NOT NULL DEFAULT 3
        CONSTRAINT quinielas_prize_threshold_positive CHECK (prize_threshold > 0);
