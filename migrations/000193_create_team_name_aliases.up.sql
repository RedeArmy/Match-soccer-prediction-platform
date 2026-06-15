-- team_name_aliases maps provider-specific team names to the canonical names
-- stored in the matches table. Required because api-football v3 uses different
-- names for some national teams (e.g. "Cabo Verde" vs our "Cape Verde").
--
-- canonical_name: the exact value stored in matches.home_team / matches.away_team.
-- provider_name:  the value returned by the external provider for the same team.
-- provider:       provider key, always "api-football" for the current integration.
--
-- FindByTeams resolves incoming provider names through this table before the
-- WHERE clause, so unlinked matches are found even when names diverge.
--
-- Sources used to verify provider names:
--   Sofascore live match data (June 15 2026, Spain vs Cape Verde in progress)
--   → confirmed: "Cabo Verde", "Congo DR", "Côte d'Ivoire"
--   Run: FOOTBALL_API_KEY=<key> go run ./cmd/check-api-teams
--   to compare all 48 team names directly against the api-football response.

CREATE TABLE IF NOT EXISTS team_name_aliases (
    id             SERIAL      PRIMARY KEY,
    canonical_name TEXT        NOT NULL,
    provider_name  TEXT        NOT NULL,
    provider       TEXT        NOT NULL DEFAULT 'api-football',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_team_alias_provider UNIQUE (provider, provider_name)
);

-- Confirmed: api-football uses Portuguese official FIFA name "Cabo Verde".
-- Sofascore (which mirrors FIFA naming) displays "Cabo Verde", not "Cape Verde".
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    ('Cape Verde',             'Cabo Verde')
ON CONFLICT (provider, provider_name) DO NOTHING;

-- Likely mismatches based on Sofascore live data (mirrors api-football FIFA naming).
-- Verify with: FOOTBALL_API_KEY=<key> go run ./cmd/check-api-teams
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    -- Sofascore shows "Congo DR" (reversed from our "DR Congo")
    ('DR Congo',               'Congo DR'),
    -- Sofascore shows "Côte d'Ivoire" (French official name vs our English "Ivory Coast")
    ('Ivory Coast',            'Côte d''Ivoire'),
    -- Extra safety: without accents in case provider strips diacritics
    ('Ivory Coast',            'Cote d''Ivoire'),
    -- Sofascore shows "Bosnia and Herzegovina" matching our DB; add & variant as safety
    ('Bosnia and Herzegovina', 'Bosnia & Herzegovina'),
    ('Bosnia and Herzegovina', 'Bosnia')
ON CONFLICT (provider, provider_name) DO NOTHING;
