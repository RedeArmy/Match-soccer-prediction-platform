-- Migration: create teams table
--
-- Stores the 48 FIFA World Cup 2026 qualified nations with confederation
-- and pre-tournament FIFA ranking for display and filtering purposes.
-- Rankings are informational; the authoritative value is the April 2026
-- FIFA ranking release and can be updated via the admin API.
--
-- iso_code uses the FIFA 3-letter code (e.g. ENG, ARG) rather than the
-- ISO 3166-1 alpha-2/3 codes because FIFA uses non-standard codes for
-- home nations (Scotland=SCO, England=ENG) that diverge from ISO.

CREATE TABLE IF NOT EXISTS teams (
    id            SERIAL      PRIMARY KEY,
    name          TEXT        NOT NULL,
    iso_code      CHAR(3)     NOT NULL,
    confederation TEXT        NOT NULL
                  CHECK (confederation IN ('UEFA','CONMEBOL','CONCACAF','CAF','AFC','OFC')),
    fifa_ranking  INTEGER,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_teams_name     UNIQUE (name),
    CONSTRAINT uq_teams_iso_code UNIQUE (iso_code)
);

CREATE INDEX IF NOT EXISTS idx_teams_confederation ON teams (confederation);
