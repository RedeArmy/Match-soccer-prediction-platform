DROP INDEX IF EXISTS uq_leaderboard_snapshots_quiniela_match;
ALTER TABLE leaderboard_snapshots DROP COLUMN IF EXISTS triggered_by_match_id;
