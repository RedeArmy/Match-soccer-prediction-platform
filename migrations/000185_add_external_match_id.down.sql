DROP INDEX IF EXISTS idx_matches_sync_candidates;
DROP INDEX IF EXISTS idx_matches_external;
ALTER TABLE matches
    DROP COLUMN IF EXISTS last_synced_at,
    DROP COLUMN IF EXISTS external_match_id,
    DROP COLUMN IF EXISTS external_provider;
