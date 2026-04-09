-- Migration: create countries, states, and cities tables
--
-- These three tables model the venue-location hierarchy required by FIFA World
-- Cup 2026: every stadium belongs to a city, every city belongs to a state or
-- province, and every state belongs to a country.
--
-- The data is purely reference: the 3 host nations, 14 host states/provinces,
-- and 16 host cities are fixed for the tournament. No CRUD endpoints are
-- exposed for these tables; they are populated once at migration time and
-- updated only when a host city changes (an extremely rare event).
--
-- Unique constraints are chosen carefully:
--   countries.code          — ISO 3166-1 alpha-2 code is globally unique.
--   states.(code,country_id)— state/province codes are unique within a country
--                             (e.g. 'CA' is both California-US and the country
--                             code for Canada, so code alone is insufficient).
--   cities.(name,state_id)  — city names are unique within a state for the
--                             16 FIFA 2026 host cities.

CREATE TABLE IF NOT EXISTS countries (
    id         SERIAL      PRIMARY KEY,
    name       TEXT        NOT NULL,
    code       CHAR(2)     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_countries_code UNIQUE (code)
);

CREATE TABLE IF NOT EXISTS states (
    id         SERIAL      PRIMARY KEY,
    name       TEXT        NOT NULL,
    code       TEXT        NOT NULL,
    country_id INTEGER     NOT NULL REFERENCES countries(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_states_code_country UNIQUE (code, country_id)
);

CREATE INDEX IF NOT EXISTS idx_states_country_id ON states (country_id);

CREATE TABLE IF NOT EXISTS cities (
    id         SERIAL      PRIMARY KEY,
    name       TEXT        NOT NULL,
    state_id   INTEGER     NOT NULL REFERENCES states(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_cities_name_state UNIQUE (name, state_id)
);

CREATE INDEX IF NOT EXISTS idx_cities_state_id ON cities (state_id);

-- ── Seed: FIFA World Cup 2026 host nations ────────────────────────────────────

INSERT INTO countries (name, code) VALUES
    ('United States', 'US'),
    ('Mexico',        'MX'),
    ('Canada',        'CA')
ON CONFLICT (code) DO NOTHING;

-- ── Seed: host states and provinces ──────────────────────────────────────────

INSERT INTO states (name, code, country_id) VALUES
    -- United States (12 host cities across 9 states)
    ('New Jersey',       'NJ',   (SELECT id FROM countries WHERE code = 'US')),
    ('Texas',            'TX',   (SELECT id FROM countries WHERE code = 'US')),
    ('California',       'CA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Florida',          'FL',   (SELECT id FROM countries WHERE code = 'US')),
    ('Pennsylvania',     'PA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Washington',       'WA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Missouri',         'MO',   (SELECT id FROM countries WHERE code = 'US')),
    ('Georgia',          'GA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Massachusetts',    'MA',   (SELECT id FROM countries WHERE code = 'US')),
    -- Mexico (3 host cities across 3 states)
    ('Ciudad de México', 'CDMX', (SELECT id FROM countries WHERE code = 'MX')),
    ('Nuevo León',       'NL',   (SELECT id FROM countries WHERE code = 'MX')),
    ('Jalisco',          'JAL',  (SELECT id FROM countries WHERE code = 'MX')),
    -- Canada (2 host cities across 2 provinces)
    ('British Columbia', 'BC',   (SELECT id FROM countries WHERE code = 'CA')),
    ('Ontario',          'ON',   (SELECT id FROM countries WHERE code = 'CA'))
ON CONFLICT (code, country_id) DO NOTHING;

-- ── Seed: host cities ─────────────────────────────────────────────────────────
-- Each subquery uses (state_code, country_code) to resolve the correct state,
-- avoiding ambiguity between identically-coded values in different countries
-- (e.g. state code 'CA' for California and country code 'CA' for Canada).

INSERT INTO cities (name, state_id) VALUES
    ('East Rutherford',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'NJ'   AND c.code = 'US')),
    ('Arlington',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'TX'   AND c.code = 'US')),
    ('Inglewood',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'CA'   AND c.code = 'US')),
    ('Miami Gardens',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'FL'   AND c.code = 'US')),
    ('Santa Clara',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'CA'   AND c.code = 'US')),
    ('Philadelphia',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'PA'   AND c.code = 'US')),
    ('Seattle',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'WA'   AND c.code = 'US')),
    ('Kansas City',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'MO'   AND c.code = 'US')),
    ('Atlanta',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'GA'   AND c.code = 'US')),
    ('Houston',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'TX'   AND c.code = 'US')),
    ('Foxborough',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'MA'   AND c.code = 'US')),
    ('Mexico City',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'CDMX' AND c.code = 'MX')),
    ('Monterrey',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'NL'   AND c.code = 'MX')),
    ('Guadalajara',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'JAL'  AND c.code = 'MX')),
    ('Vancouver',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'BC'   AND c.code = 'CA')),
    ('Toronto',
        (SELECT s.id FROM states s JOIN countries c ON s.country_id = c.id WHERE s.code = 'ON'   AND c.code = 'CA'))
ON CONFLICT (name, state_id) DO NOTHING;
