-- Migration: add match_code to matches
--
-- match_code stores the official FIFA match number (e.g. 'M1' … 'M104').
-- Nullable so that matches created via the admin API before this migration
-- runs, or on-the-fly non-tournament fixtures, need no code assigned.
-- The unique constraint prevents duplicate codes while still allowing
-- multiple NULL values (PostgreSQL treats each NULL as distinct).

ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS match_code VARCHAR(10);

ALTER TABLE matches
    ADD CONSTRAINT uq_matches_match_code UNIQUE (match_code);
