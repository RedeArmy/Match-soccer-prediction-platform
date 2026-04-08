-- Migration: add performance indexes for high-frequency query paths
--
-- Most indexes (matches.status, matches.phase, predictions.user_id,
-- predictions.match_id, quinielas.owner_id) were created alongside their
-- respective tables. This migration fills the two remaining gaps identified
-- by query analysis.
--
-- Naming convention: idx_<table>_<column(s)>. The existing matches_phase_idx
-- predates this convention; new indexes follow the consistent prefix style.

-- matches(kickoff_at DESC)
--
-- ListMatches returns fixtures ordered by kickoff time (most recent first).
-- Without an index PostgreSQL performs a sequential scan followed by an
-- in-memory sort. As the schedule grows to 104 matches (FIFA 2026 format)
-- this sort is cheap, but the index becomes important once range predicates
-- (e.g. upcoming matches only) are layered on top.
-- DESC matches the ORDER BY direction used in the list query so the planner
-- can satisfy the sort without an additional sort step.
CREATE INDEX IF NOT EXISTS idx_matches_kickoff_at ON matches (kickoff_at DESC);

-- tiebreakers(quiniela_id)
--
-- ListByQuiniela filters solely by quiniela_id. The existing unique constraint
-- on (user_id, quiniela_id) is a composite index whose leftmost column is
-- user_id; PostgreSQL cannot use that index efficiently for a predicate on
-- quiniela_id alone. This dedicated index closes that gap.
CREATE INDEX IF NOT EXISTS idx_tiebreakers_quiniela_id ON tiebreakers (quiniela_id);
