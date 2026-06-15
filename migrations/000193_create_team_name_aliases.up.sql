-- team_name_aliases maps provider-specific team names to the canonical names
-- stored in the matches table. Required because api-football v3 uses different
-- names for some national teams.
--
-- canonical_name: the exact value stored in matches.home_team / matches.away_team.
-- provider_name:  the value returned by the external provider for the same team.
-- provider:       provider key, always "api-football" for the current integration.
--
-- Verified against all 72 fixtures for the 2026 FIFA World Cup via live API
-- call on 2026-06-15. All 48 teams were compared; only 4 names differ between
-- the api-football response and the canonical names in the matches table.

CREATE TABLE IF NOT EXISTS team_name_aliases (
    id             SERIAL      PRIMARY KEY,
    canonical_name TEXT        NOT NULL,
    provider_name  TEXT        NOT NULL,
    provider       TEXT        NOT NULL DEFAULT 'api-football',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_team_alias_provider UNIQUE (provider, provider_name)
);

-- All 4 confirmed mismatches for the 2026 FIFA World Cup (api-football v3).
-- The remaining 44 teams match their DB canonical names exactly.
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    -- api-football uses the full official FIFA name with "Islands"
    ('Cape Verde',             'Cape Verde Islands'),
    -- api-football reverses the word order vs our canonical "DR Congo"
    ('DR Congo',               'Congo DR'),
    -- api-football uses "&" while our DB stores the full conjunction "and"
    ('Bosnia and Herzegovina', 'Bosnia & Herzegovina'),
    -- api-football uses the abbreviation; our DB stores the full English name
    ('United States',          'USA')
ON CONFLICT (provider, provider_name) DO NOTHING;
