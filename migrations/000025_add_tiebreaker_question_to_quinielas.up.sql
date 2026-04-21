-- Migration: create tiebreaker_config table
--
-- Stores the single global tiebreaker question and the confirmed result.
-- Only one row is ever present (upserted on id=1). The system administrator
-- defines the question; members submit their numeric estimates; the
-- administrator later confirms the actual result.
CREATE TABLE IF NOT EXISTS tiebreaker_config (
    id         SERIAL      PRIMARY KEY,
    question   TEXT        NOT NULL,
    result     INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
