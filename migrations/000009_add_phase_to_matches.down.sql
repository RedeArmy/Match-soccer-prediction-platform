DROP INDEX IF EXISTS matches_phase_idx;

ALTER TABLE matches DROP COLUMN IF EXISTS phase;
