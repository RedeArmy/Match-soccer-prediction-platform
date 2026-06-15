-- team_name_aliases maps provider-specific team names to the canonical names
-- stored in the matches table. Required because api-football v3 uses different
-- names for some national teams.
--
-- canonical_name: the exact value stored in matches.home_team / matches.away_team.
-- provider_name:  the value returned by the external provider for the same team.
-- provider:       provider key, always "api-football" for the current integration.
--
-- FindByTeams resolves incoming provider names through this table before the
-- WHERE clause, so unlinked matches are found even when names diverge.
--
-- Verified with: FOOTBALL_API_KEY=<key> go run ./cmd/check-api-teams
-- Run against live 2026 World Cup fixtures on 2026-06-15.

CREATE TABLE IF NOT EXISTS team_name_aliases (
    id             SERIAL      PRIMARY KEY,
    canonical_name TEXT        NOT NULL,
    provider_name  TEXT        NOT NULL,
    provider       TEXT        NOT NULL DEFAULT 'api-football',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_team_alias_provider UNIQUE (provider, provider_name)
);

-- ── Confirmed mismatches (verified via FOOTBALL_API_KEY live check, 2026-06-15)
-- api-football uses the full official FIFA name "Cape Verde Islands" whereas
-- our matches table stores the common short form "Cape Verde".
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    ('Cape Verde', 'Cape Verde Islands')
ON CONFLICT (provider, provider_name) DO NOTHING;

-- ── Defensive aliases for teams not yet seen in live fixtures ─────────────────
-- These are based on known api-football naming patterns for teams whose matches
-- fall outside the today/tomorrow window of cmd/check-api-teams.
-- Re-verify with the tool once those fixtures are scheduled.
INSERT INTO team_name_aliases (canonical_name, provider_name) VALUES
    -- Congo DR: Sofascore (which mirrors api-football naming) shows "Congo DR"
    ('DR Congo',               'Congo DR'),
    -- Ivory Coast: api-football may use the French FIFA name on some records
    ('Ivory Coast',            'Côte d''Ivoire'),
    ('Ivory Coast',            'Cote d''Ivoire'),
    -- Bosnia variants: FIFA uses the full name; providers often abbreviate
    ('Bosnia and Herzegovina', 'Bosnia & Herzegovina'),
    ('Bosnia and Herzegovina', 'Bosnia'),
    -- Czech Republic: FIFA now uses Czechia but some provider records are stale
    ('Czechia',                'Czech Republic'),
    -- United States: occasionally abbreviated in older provider records
    ('United States',          'USA'),
    -- South Korea: FIFA official name is "Korea Republic"
    ('South Korea',            'Korea Republic')
ON CONFLICT (provider, provider_name) DO NOTHING;
