-- Migration: replace single-column matches indexes with composite covering indexes
--
-- ListByStatus and ListByPhase both filter on a single column and then order
-- by kickoff_at DESC. With separate indexes the planner must choose between
-- using the filter index (fast filter, slow sort) or the kickoff_at index
-- (fast sort, slow filter via index scan + recheck). Neither option is optimal.
--
-- Composite indexes on (status, kickoff_at DESC) and (phase, kickoff_at DESC)
-- let PostgreSQL satisfy the WHERE predicate and the ORDER BY in a single
-- index scan with no additional sort step. The leading column is the equality
-- predicate; the trailing column provides pre-sorted order.
--
-- The standalone idx_matches_status and idx_matches_phase indexes become
-- redundant once the composite indexes exist: any query that previously used
-- them will now prefer the composite version. Dropping them reduces index
-- maintenance overhead on INSERT and UPDATE.
--
-- idx_matches_kickoff_at is retained for ListMatches (no filter, order only).

DROP INDEX IF EXISTS idx_matches_status;
DROP INDEX IF EXISTS idx_matches_phase;

CREATE INDEX IF NOT EXISTS idx_matches_status_kickoff
    ON matches (status, kickoff_at DESC);

CREATE INDEX IF NOT EXISTS idx_matches_phase_kickoff
    ON matches (phase, kickoff_at DESC);
