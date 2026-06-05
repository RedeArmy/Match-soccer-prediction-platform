-- Seed: FIFA World Cup 2026 qualified nations (48 teams)
--
-- Sources: FIFA.com official team list, draw results (December 5, 2025),
--          Yahoo Sports "2026 World Cup schedule qualified teams groups"
--
-- iso_code: FIFA 3-letter code (diverges from ISO 3166-1 alpha-3 for home
--   nations: England=ENG, Scotland=SCO, etc.)
-- fifa_ranking: approximate April 2026 FIFA ranking; informational only —
--   update via PATCH /admin/teams/{id} when official rankings are published.
-- ON CONFLICT (name) DO NOTHING ensures idempotency on repeated runs.

INSERT INTO teams (name, iso_code, confederation, fifa_ranking)
VALUES
    -- Group A -----------------------------------------------------------------
    ('Mexico',                  'MEX', 'CONCACAF',  20),
    ('South Africa',            'RSA', 'CAF',        34),
    ('South Korea',             'KOR', 'AFC',        19),
    ('Czechia',                 'CZE', 'UEFA',       30),
    -- Group B -----------------------------------------------------------------
    ('Canada',                  'CAN', 'CONCACAF',  37),
    ('Bosnia and Herzegovina',  'BIH', 'UEFA',       38),
    ('Qatar',                   'QAT', 'AFC',        42),
    ('Switzerland',             'SUI', 'UEFA',       17),
    -- Group C -----------------------------------------------------------------
    ('Brazil',                  'BRA', 'CONMEBOL',   4),
    ('Morocco',                 'MAR', 'CAF',        12),
    ('Haiti',                   'HAI', 'CONCACAF',  46),
    ('Scotland',                'SCO', 'UEFA',       29),
    -- Group D -----------------------------------------------------------------
    ('United States',           'USA', 'CONCACAF',  11),
    ('Paraguay',                'PAR', 'CONMEBOL',  48),
    ('Australia',               'AUS', 'AFC',        26),
    ('Türkiye',                 'TUR', 'UEFA',       23),
    -- Group E -----------------------------------------------------------------
    ('Germany',                 'GER', 'UEFA',        9),
    ('Curaçao',                 'CUW', 'CONCACAF',  47),
    ('Ivory Coast',             'CIV', 'CAF',        31),
    ('Ecuador',                 'ECU', 'CONMEBOL',  22),
    -- Group F -----------------------------------------------------------------
    ('Netherlands',             'NED', 'UEFA',        8),
    ('Japan',                   'JPN', 'AFC',        14),
    ('Sweden',                  'SWE', 'UEFA',       24),
    ('Tunisia',                 'TUN', 'CAF',        32),
    -- Group G -----------------------------------------------------------------
    ('Belgium',                 'BEL', 'UEFA',        6),
    ('Egypt',                   'EGY', 'CAF',        28),
    ('Iran',                    'IRN', 'AFC',        27),
    ('New Zealand',             'NZL', 'OFC',        41),
    -- Group H -----------------------------------------------------------------
    ('Spain',                   'ESP', 'UEFA',        5),
    ('Cape Verde',              'CPV', 'CAF',        45),
    ('Saudi Arabia',            'KSA', 'AFC',        35),
    ('Uruguay',                 'URU', 'CONMEBOL',  15),
    -- Group I -----------------------------------------------------------------
    ('France',                  'FRA', 'UEFA',        2),
    ('Senegal',                 'SEN', 'CAF',        18),
    ('Iraq',                    'IRQ', 'AFC',        43),
    ('Norway',                  'NOR', 'UEFA',       21),
    -- Group J -----------------------------------------------------------------
    ('Argentina',               'ARG', 'CONMEBOL',   1),
    ('Algeria',                 'ALG', 'CAF',        25),
    ('Austria',                 'AUT', 'UEFA',       16),
    ('Jordan',                  'JOR', 'AFC',        40),
    -- Group K -----------------------------------------------------------------
    ('Portugal',                'POR', 'UEFA',        7),
    ('DR Congo',                'COD', 'CAF',        39),
    ('Uzbekistan',              'UZB', 'AFC',        44),
    ('Colombia',                'COL', 'CONMEBOL',  10),
    -- Group L -----------------------------------------------------------------
    ('England',                 'ENG', 'UEFA',        3),
    ('Croatia',                 'CRO', 'UEFA',       13),
    ('Ghana',                   'GHA', 'CAF',        33),
    ('Panama',                  'PAN', 'CONCACAF',  36)
ON CONFLICT (name) DO NOTHING;
