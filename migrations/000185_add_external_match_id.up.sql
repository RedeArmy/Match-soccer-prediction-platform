-- Migration 000185: external match identifier for automated result sync
-- Stores the API-Football fixture ID so the match-sync worker can poll
-- each fixture by its provider-assigned ID without a full-table scan.
-- external_provider is a free-form label ("api-football") so the schema
-- can accommodate additional data sources without a new migration.
ALTER TABLE matches
    ADD COLUMN external_provider  VARCHAR(64),
    ADD COLUMN external_match_id  BIGINT,
    ADD COLUMN last_synced_at     TIMESTAMPTZ;

-- Unique partial index: one external ID per provider, only for linked rows.
CREATE UNIQUE INDEX idx_matches_external
    ON matches (external_provider, external_match_id)
    WHERE external_match_id IS NOT NULL;

-- Partial index used by the sync worker to efficiently fetch all rows
-- that have an external link and are not yet finished.
CREATE INDEX idx_matches_sync_candidates
    ON matches (external_match_id)
    WHERE external_match_id IS NOT NULL
      AND status IN ('scheduled', 'live');
