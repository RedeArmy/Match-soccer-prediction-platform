-- Seed: FIFA World Cup 2026 group stage fixtures (M1–M72, 72 matches)
--
-- Sources:
--   ESPN "2026 FIFA World Cup fixtures and results" (espn.com/soccer/story/_/id/48939282)
--   NBC Sports "2026 World Cup schedule confirmed dates times stadiums"
--   Yahoo Sports "2026 World Cup schedule qualified teams groups"
--
-- All kickoff times are UTC.  Mexico City/Monterrey/Guadalajara are on CST
-- (UTC-6, permanent since Mexico abolished DST in 2022).  US eastern venues
-- are EDT (UTC-4); central are CDT (UTC-5); Pacific are PDT (UTC-7).
-- Toronto is EDT (UTC-4); Vancouver is PDT (UTC-7).
--
-- Round 3 pairs for each group are inserted at the same UTC timestamp to
-- enforce simultaneous kickoffs (FIFA fairness requirement).
--
-- stadium_id is resolved by a scalar subquery on the unique stadium name;
-- if a venue is absent the field falls back to NULL (allowed by schema).
-- ON CONFLICT (home_team, away_team, kickoff_at) DO NOTHING ensures
-- idempotency on repeated runs.

INSERT INTO matches
    (home_team, away_team, status, phase, group_label, stadium_id, kickoff_at, match_code)
SELECT
    v.home_team,
    v.away_team,
    'scheduled',
    'group_stage',
    v.grp,
    (SELECT id FROM stadiums WHERE name = v.venue),
    v.ko::timestamptz,
    v.mc
FROM (VALUES
    -- ── Round 1 ─────────────────────────────────────────────────────────────
    -- Group A
    ('Mexico',                 'South Africa',          'A', 'Estadio Azteca',          '2026-06-11 19:00:00+00', 'M1' ),
    ('South Korea',            'Czechia',               'A', 'Estadio Akron',           '2026-06-12 02:00:00+00', 'M2' ),
    -- Group B
    ('Canada',                 'Bosnia and Herzegovina','B', 'BMO Field',               '2026-06-12 19:00:00+00', 'M3' ),
    -- Group D  (USA home opener at SoFi, 6 pm PDT = 01:00 UTC Jun 13)
    ('United States',          'Paraguay',              'D', 'SoFi Stadium',            '2026-06-13 01:00:00+00', 'M4' ),
    -- Group B
    ('Qatar',                  'Switzerland',           'B', 'Levi''s Stadium',         '2026-06-13 19:00:00+00', 'M5' ),
    -- Group C
    ('Brazil',                 'Morocco',               'C', 'MetLife Stadium',         '2026-06-13 22:00:00+00', 'M6' ),
    ('Haiti',                  'Scotland',              'C', 'Gillette Stadium',        '2026-06-14 01:00:00+00', 'M7' ),
    -- Group D
    ('Australia',              'Türkiye',               'D', 'BC Place',                '2026-06-14 04:00:00+00', 'M8' ),
    -- Group E
    ('Germany',                'Curaçao',               'E', 'NRG Stadium',             '2026-06-14 18:00:00+00', 'M9' ),
    -- Group F
    ('Netherlands',            'Japan',                 'F', 'AT&T Stadium',            '2026-06-14 20:00:00+00', 'M10'),
    -- Group E
    ('Ivory Coast',            'Ecuador',               'E', 'Lincoln Financial Field', '2026-06-15 00:00:00+00', 'M11'),
    -- Group F
    ('Sweden',                 'Tunisia',               'F', 'Estadio BBVA',            '2026-06-15 02:00:00+00', 'M12'),
    -- Group H
    ('Spain',                  'Cape Verde',            'H', 'Mercedes-Benz Stadium',  '2026-06-15 17:00:00+00', 'M13'),
    -- Group G
    ('Belgium',                'Egypt',                 'G', 'Lumen Field',             '2026-06-15 21:00:00+00', 'M14'),
    -- Group H
    ('Saudi Arabia',           'Uruguay',               'H', 'Hard Rock Stadium',       '2026-06-16 00:00:00+00', 'M15'),
    -- Group G
    ('Iran',                   'New Zealand',           'G', 'SoFi Stadium',            '2026-06-16 04:00:00+00', 'M16'),
    -- Group I
    ('France',                 'Senegal',               'I', 'MetLife Stadium',         '2026-06-16 19:00:00+00', 'M17'),
    ('Iraq',                   'Norway',                'I', 'Gillette Stadium',        '2026-06-16 22:00:00+00', 'M18'),
    -- Group J
    ('Argentina',              'Algeria',               'J', 'Arrowhead Stadium',       '2026-06-17 01:00:00+00', 'M19'),
    ('Austria',                'Jordan',                'J', 'Levi''s Stadium',         '2026-06-17 04:00:00+00', 'M20'),
    -- Group K
    ('Portugal',               'DR Congo',              'K', 'NRG Stadium',             '2026-06-17 17:00:00+00', 'M21'),
    -- Group L
    ('England',                'Croatia',               'L', 'AT&T Stadium',            '2026-06-17 20:00:00+00', 'M22'),
    ('Ghana',                  'Panama',                'L', 'BMO Field',               '2026-06-18 00:00:00+00', 'M23'),
    -- Group K
    ('Uzbekistan',             'Colombia',              'K', 'Estadio Azteca',          '2026-06-18 02:00:00+00', 'M24'),

    -- ── Round 2 ─────────────────────────────────────────────────────────────
    -- Group A
    ('Czechia',                'South Africa',          'A', 'Mercedes-Benz Stadium',  '2026-06-18 16:00:00+00', 'M25'),
    -- Group B
    ('Switzerland',            'Bosnia and Herzegovina','B', 'SoFi Stadium',            '2026-06-18 19:00:00+00', 'M26'),
    ('Canada',                 'Qatar',                 'B', 'BC Place',                '2026-06-18 22:00:00+00', 'M27'),
    -- Group A
    ('Mexico',                 'South Korea',           'A', 'Estadio Akron',           '2026-06-19 03:00:00+00', 'M28'),
    -- Group D
    ('United States',          'Australia',             'D', 'Lumen Field',             '2026-06-19 19:00:00+00', 'M29'),
    -- Group C
    ('Scotland',               'Morocco',               'C', 'Gillette Stadium',        '2026-06-19 22:00:00+00', 'M30'),
    ('Brazil',                 'Haiti',                 'C', 'Lincoln Financial Field', '2026-06-20 01:00:00+00', 'M31'),
    -- Group D
    ('Türkiye',                'Paraguay',              'D', 'Levi''s Stadium',         '2026-06-20 04:00:00+00', 'M32'),
    -- Group F
    ('Netherlands',            'Sweden',                'F', 'NRG Stadium',             '2026-06-20 18:00:00+00', 'M33'),
    -- Group E
    ('Germany',                'Ivory Coast',           'E', 'BMO Field',               '2026-06-20 20:00:00+00', 'M34'),
    -- Group E
    ('Ecuador',                'Curaçao',               'E', 'Arrowhead Stadium',       '2026-06-21 01:00:00+00', 'M35'),
    -- Group F
    ('Tunisia',                'Japan',                 'F', 'Estadio BBVA',            '2026-06-21 04:00:00+00', 'M36'),
    -- Group H
    ('Spain',                  'Saudi Arabia',          'H', 'Mercedes-Benz Stadium',  '2026-06-21 16:00:00+00', 'M37'),
    -- Group G
    ('Belgium',                'Iran',                  'G', 'SoFi Stadium',            '2026-06-21 19:00:00+00', 'M38'),
    -- Group H
    ('Uruguay',                'Cape Verde',            'H', 'Hard Rock Stadium',       '2026-06-22 00:00:00+00', 'M39'),
    -- Group G
    ('New Zealand',            'Egypt',                 'G', 'BC Place',                '2026-06-22 01:00:00+00', 'M40'),
    -- Group J
    ('Argentina',              'Austria',               'J', 'AT&T Stadium',            '2026-06-22 17:00:00+00', 'M41'),
    -- Group I
    ('France',                 'Iraq',                  'I', 'Lincoln Financial Field', '2026-06-22 21:00:00+00', 'M42'),
    ('Norway',                 'Senegal',               'I', 'MetLife Stadium',         '2026-06-23 00:00:00+00', 'M43'),
    -- Group J
    ('Jordan',                 'Algeria',               'J', 'Levi''s Stadium',         '2026-06-23 03:00:00+00', 'M44'),
    -- Group K
    ('Portugal',               'Uzbekistan',            'K', 'NRG Stadium',             '2026-06-23 17:00:00+00', 'M45'),
    -- Group L
    ('England',                'Ghana',                 'L', 'Gillette Stadium',        '2026-06-23 20:00:00+00', 'M46'),
    ('Panama',                 'Croatia',               'L', 'BMO Field',               '2026-06-23 23:00:00+00', 'M47'),
    -- Group K
    ('Colombia',               'DR Congo',              'K', 'Estadio Akron',           '2026-06-24 02:00:00+00', 'M48'),

    -- ── Round 3 (simultaneous within each group) ─────────────────────────────
    -- Group B — both at 19:00 UTC Jun 24
    ('Switzerland',            'Canada',                'B', 'BC Place',                '2026-06-24 19:00:00+00', 'M49'),
    ('Bosnia and Herzegovina', 'Qatar',                 'B', 'Lumen Field',             '2026-06-24 19:00:00+00', 'M50'),
    -- Group C — both at 22:00 UTC Jun 24
    ('Scotland',               'Brazil',                'C', 'Hard Rock Stadium',       '2026-06-24 22:00:00+00', 'M51'),
    ('Morocco',                'Haiti',                 'C', 'Mercedes-Benz Stadium',  '2026-06-24 22:00:00+00', 'M52'),
    -- Group A — both at 01:00 UTC Jun 25 (7 pm CST Jun 24)
    ('Czechia',                'Mexico',                'A', 'Estadio Azteca',          '2026-06-25 01:00:00+00', 'M53'),
    ('South Africa',           'South Korea',           'A', 'Estadio Akron',           '2026-06-25 01:00:00+00', 'M54'),
    -- Group E — both at 20:00 UTC Jun 25
    ('Ecuador',                'Germany',               'E', 'MetLife Stadium',         '2026-06-25 20:00:00+00', 'M55'),
    ('Curaçao',                'Ivory Coast',           'E', 'Lincoln Financial Field', '2026-06-25 20:00:00+00', 'M56'),
    -- Group F — both at 21:00 UTC Jun 25
    ('Japan',                  'Sweden',                'F', 'AT&T Stadium',            '2026-06-25 21:00:00+00', 'M57'),
    ('Tunisia',                'Netherlands',           'F', 'Arrowhead Stadium',       '2026-06-25 21:00:00+00', 'M58'),
    -- Group D — both at 01:00 UTC Jun 26 (6 pm PDT Jun 25)
    ('Türkiye',                'United States',         'D', 'SoFi Stadium',            '2026-06-26 01:00:00+00', 'M59'),
    ('Paraguay',               'Australia',             'D', 'Levi''s Stadium',         '2026-06-26 01:00:00+00', 'M60'),
    -- Group I — both at 19:00 UTC Jun 26
    ('Norway',                 'France',                'I', 'Gillette Stadium',        '2026-06-26 19:00:00+00', 'M61'),
    ('Senegal',                'Iraq',                  'I', 'BMO Field',               '2026-06-26 19:00:00+00', 'M62'),
    -- Group H — both at 23:00 UTC Jun 26 (6 pm CDT Houston / 5 pm CST Guadalajara)
    ('Cape Verde',             'Saudi Arabia',          'H', 'NRG Stadium',             '2026-06-26 23:00:00+00', 'M63'),
    ('Uruguay',                'Spain',                 'H', 'Estadio Akron',           '2026-06-26 23:00:00+00', 'M64'),
    -- Group G — both at 03:00 UTC Jun 27 (11 pm EDT Jun 26)
    ('Egypt',                  'Iran',                  'G', 'Lumen Field',             '2026-06-27 03:00:00+00', 'M65'),
    ('New Zealand',            'Belgium',               'G', 'BC Place',                '2026-06-27 03:00:00+00', 'M66'),
    -- Group L — both at 21:00 UTC Jun 27 (5 pm EDT)
    ('Panama',                 'England',               'L', 'MetLife Stadium',         '2026-06-27 21:00:00+00', 'M71'),
    ('Croatia',                'Ghana',                 'L', 'Lincoln Financial Field', '2026-06-27 21:00:00+00', 'M72'),
    -- Group K — both at 23:30 UTC Jun 27 (7:30 pm EDT)
    ('Colombia',               'Portugal',              'K', 'Hard Rock Stadium',       '2026-06-27 23:30:00+00', 'M69'),
    ('DR Congo',               'Uzbekistan',            'K', 'Mercedes-Benz Stadium',  '2026-06-27 23:30:00+00', 'M70'),
    -- Group J — both at 02:00 UTC Jun 28 (10 pm EDT Jun 27)
    ('Algeria',                'Austria',               'J', 'Arrowhead Stadium',       '2026-06-28 02:00:00+00', 'M67'),
    ('Jordan',                 'Argentina',             'J', 'AT&T Stadium',            '2026-06-28 02:00:00+00', 'M68')
) AS v(home_team, away_team, grp, venue, ko, mc)
ON CONFLICT (home_team, away_team, kickoff_at) DO NOTHING;
