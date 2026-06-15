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
-- Verification: FOOTBALL_API_KEY=<key> go run ./cmd/check-api-teams
-- compares all 48 team names directly against the api-football response.

CREATE TABLE IF NOT EXISTS team_name_aliases (
    id             SERIAL      PRIMARY KEY,
    canonical_name TEXT        NOT NULL,
    provider_name  TEXT        NOT NULL,
    provider       TEXT        NOT NULL DEFAULT 'api-football',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_team_alias_provider UNIQUE (provider, provider_name)
);

-- ── Confirmed mismatches (verified via Sofascore live data, June 2026) ────────
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    -- api-football uses Portuguese/FIFA official name
    ('Cape Verde',             'Cabo Verde'),
    -- api-football reverses the word order from FIFA official "DR Congo"
    ('DR Congo',               'Congo DR'),
    -- api-football uses French official FIFA name
    ('Ivory Coast',            'Côte d''Ivoire'),
    -- same without diacritics (some API responses strip accents)
    ('Ivory Coast',            'Cote d''Ivoire'),
    -- Bosnia variants: FIFA uses the full name; providers often abbreviate
    ('Bosnia and Herzegovina', 'Bosnia & Herzegovina'),
    ('Bosnia and Herzegovina', 'Bosnia')
ON CONFLICT (provider, provider_name) DO NOTHING;

-- ── Likely mismatches based on known api-football v3 naming conventions ───────
-- These teams were renamed by FIFA but api-football may still return the old
-- English name.  Verify with: go run ./cmd/check-api-teams
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    -- FIFA renamed Turkey → Türkiye in April 2022; some api-football records
    -- still use the English name "Turkey"
    ('Türkiye',                'Turkey'),
    -- FIFA's shorthand is "Czechia"; api-football may return "Czech Republic"
    ('Czechia',                'Czech Republic'),
    -- Curaçao: accent often stripped in ASCII-only API responses
    ('Curaçao',                'Curacao'),
    -- United States: api-football sometimes returns the abbreviation
    ('United States',          'USA'),
    -- South Korea: FIFA uses "Korea Republic" as the official name
    ('South Korea',            'Korea Republic'),
    -- New Zealand: FIFA uses "New Zealand" but some providers say "NZ"
    ('New Zealand',            'NZ'),
    -- Saudi Arabia: occasionally abbreviated
    ('Saudi Arabia',           'KSA'),
    -- Scotland/Wales/Northern Ireland sometimes listed under Great Britain umbrella
    ('England',                'ENG')
ON CONFLICT (provider, provider_name) DO NOTHING;
