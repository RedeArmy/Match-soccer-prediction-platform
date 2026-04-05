-- Migration: create matches table
--
-- The status column uses a check constraint so that invalid status values are
-- rejected at the database level, providing a second line of defence behind
-- the application-layer MatchStatus typed string.
-- An index on status is included because ListByStatus is a first-class query
-- path and a full-table scan would degrade under load.
CREATE TABLE IF NOT EXISTS matches (
    id          SERIAL PRIMARY KEY,
    home_team   TEXT        NOT NULL,
    away_team   TEXT        NOT NULL,
    home_score  INTEGER,
    away_score  INTEGER,
    status      TEXT        NOT NULL DEFAULT 'scheduled'
                CHECK (status IN ('scheduled', 'live', 'finished')),
    kickoff_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_matches_status ON matches (status);
