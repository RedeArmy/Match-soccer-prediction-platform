-- Revert matches index optimizations.
--
-- Restore the original single-column indexes and drop the composite
-- indexes introduced in the up migration.

DROP INDEX IF EXISTS idx_matches_status_kickoff;
DROP INDEX IF EXISTS idx_matches_phase_kickoff;

CREATE INDEX IF NOT EXISTS idx_matches_status ON matches (status);
CREATE INDEX IF NOT EXISTS idx_matches_phase  ON matches (phase);
