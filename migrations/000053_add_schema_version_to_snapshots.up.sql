-- Add schema_version to leaderboard_snapshots so the application can branch
-- on the JSONB encoding format when the snapshot struct changes.
--
-- DEFAULT 1 retroactively classifies all existing rows as schema version 1
-- (the original format: PascalCase field names, no explicit JSON tags).
-- No data migration is required: the existing JSONB is valid V1 content.
ALTER TABLE leaderboard_snapshots
    ADD COLUMN schema_version SMALLINT NOT NULL DEFAULT 1;
