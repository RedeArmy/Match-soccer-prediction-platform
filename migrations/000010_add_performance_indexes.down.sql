-- Rollback: remove performance indexes added in 000010
--
-- Dropping indexes is safe at any time: the tables and their data are
-- unaffected. The only consequence is slower queries on the affected
-- columns until the up migration is re-applied.
DROP INDEX IF EXISTS idx_matches_kickoff_at;
DROP INDEX IF EXISTS idx_tiebreakers_quiniela_id;
