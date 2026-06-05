-- Seed: host-nation location hierarchy (countries → states → cities → stadiums)
-- and update matches.stadium_id for all 104 FIFA World Cup 2026 fixtures.
--
-- This migration is self-contained and idempotent:
--   • INSERT … ON CONFLICT DO NOTHING on all four tables.
--   • UPDATE matches … WHERE stadium_id IS NULL at the end.
--
-- All 16 FIFA 2026 host venues across USA (11), Mexico (3), and Canada (2).
-- Sources: FIFA.com/en/tournaments/mens/worldcup/canadamexicousa2026/stadiums
--          Wikipedia "2026 FIFA World Cup venues"

-- ── 1. Countries ─────────────────────────────────────────────────────────────

INSERT INTO countries (name, code) VALUES
    ('United States', 'US'),
    ('Mexico',        'MX'),
    ('Canada',        'CA')
ON CONFLICT (code) DO NOTHING;

-- ── 2. States / provinces ────────────────────────────────────────────────────

INSERT INTO states (name, code, country_id) VALUES
    ('New Jersey',       'NJ',   (SELECT id FROM countries WHERE code = 'US')),
    ('Texas',            'TX',   (SELECT id FROM countries WHERE code = 'US')),
    ('California',       'CA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Florida',          'FL',   (SELECT id FROM countries WHERE code = 'US')),
    ('Pennsylvania',     'PA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Washington',       'WA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Missouri',         'MO',   (SELECT id FROM countries WHERE code = 'US')),
    ('Georgia',          'GA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Massachusetts',    'MA',   (SELECT id FROM countries WHERE code = 'US')),
    ('Ciudad de México', 'CDMX', (SELECT id FROM countries WHERE code = 'MX')),
    ('Nuevo León',       'NL',   (SELECT id FROM countries WHERE code = 'MX')),
    ('Jalisco',          'JAL',  (SELECT id FROM countries WHERE code = 'MX')),
    ('British Columbia', 'BC',   (SELECT id FROM countries WHERE code = 'CA')),
    ('Ontario',          'ON',   (SELECT id FROM countries WHERE code = 'CA'))
ON CONFLICT (code, country_id) DO NOTHING;

-- ── 3. Cities ─────────────────────────────────────────────────────────────────

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

-- ── 4. Stadiums ───────────────────────────────────────────────────────────────
-- city_id is resolved via a JOIN through the location hierarchy just seeded.

INSERT INTO stadiums (name, capacity, city_id)
SELECT v.name, v.capacity, ci.id
FROM (VALUES
    ('MetLife Stadium',          82500, 'East Rutherford', 'NJ',   'US'),
    ('AT&T Stadium',             80000, 'Arlington',       'TX',   'US'),
    ('SoFi Stadium',             70240, 'Inglewood',       'CA',   'US'),
    ('Hard Rock Stadium',        65326, 'Miami Gardens',   'FL',   'US'),
    ('Levi''s Stadium',          68500, 'Santa Clara',     'CA',   'US'),
    ('Lincoln Financial Field',  69796, 'Philadelphia',    'PA',   'US'),
    ('Lumen Field',              68740, 'Seattle',         'WA',   'US'),
    ('Arrowhead Stadium',        76416, 'Kansas City',     'MO',   'US'),
    ('Mercedes-Benz Stadium',    71000, 'Atlanta',         'GA',   'US'),
    ('NRG Stadium',              72220, 'Houston',         'TX',   'US'),
    ('Gillette Stadium',         65878, 'Foxborough',      'MA',   'US'),
    ('Estadio Azteca',           87523, 'Mexico City',     'CDMX', 'MX'),
    ('Estadio BBVA',             53500, 'Monterrey',       'NL',   'MX'),
    ('Estadio Akron',            49850, 'Guadalajara',     'JAL',  'MX'),
    ('BC Place',                 54500, 'Vancouver',       'BC',   'CA'),
    ('BMO Field',                45736, 'Toronto',         'ON',   'CA')
) AS v(name, capacity, city_name, state_code, country_code)
JOIN cities    ci ON ci.name         = v.city_name
JOIN states    st ON st.id           = ci.state_id   AND st.code       = v.state_code
JOIN countries co ON co.id           = st.country_id AND co.code::text = v.country_code
ON CONFLICT (name) DO NOTHING;

-- ── 5. Link matches → stadiums ────────────────────────────────────────────────
-- Updates stadium_id for every match whose venue name appears in stadiums.
-- Uses the match_code → venue mapping from migrations 000168 and 000169.
-- Matches that already have stadium_id set are left untouched (WHERE m.stadium_id IS NULL).

UPDATE matches m
SET    stadium_id = s.id
FROM   stadiums s
WHERE  m.stadium_id IS NULL
  AND  s.name = CASE m.match_code
    -- Group stage (M1–M72) ─────────────────────────────────────
    WHEN 'M1'  THEN 'Estadio Azteca'
    WHEN 'M2'  THEN 'Estadio Akron'
    WHEN 'M3'  THEN 'BMO Field'
    WHEN 'M4'  THEN 'SoFi Stadium'
    WHEN 'M5'  THEN 'Levi''s Stadium'
    WHEN 'M6'  THEN 'MetLife Stadium'
    WHEN 'M7'  THEN 'Gillette Stadium'
    WHEN 'M8'  THEN 'BC Place'
    WHEN 'M9'  THEN 'NRG Stadium'
    WHEN 'M10' THEN 'AT&T Stadium'
    WHEN 'M11' THEN 'Lincoln Financial Field'
    WHEN 'M12' THEN 'Estadio BBVA'
    WHEN 'M13' THEN 'Mercedes-Benz Stadium'
    WHEN 'M14' THEN 'Lumen Field'
    WHEN 'M15' THEN 'Hard Rock Stadium'
    WHEN 'M16' THEN 'SoFi Stadium'
    WHEN 'M17' THEN 'MetLife Stadium'
    WHEN 'M18' THEN 'Gillette Stadium'
    WHEN 'M19' THEN 'Arrowhead Stadium'
    WHEN 'M20' THEN 'Levi''s Stadium'
    WHEN 'M21' THEN 'NRG Stadium'
    WHEN 'M22' THEN 'AT&T Stadium'
    WHEN 'M23' THEN 'BMO Field'
    WHEN 'M24' THEN 'Estadio Azteca'
    WHEN 'M25' THEN 'Mercedes-Benz Stadium'
    WHEN 'M26' THEN 'SoFi Stadium'
    WHEN 'M27' THEN 'BC Place'
    WHEN 'M28' THEN 'Estadio Akron'
    WHEN 'M29' THEN 'Lumen Field'
    WHEN 'M30' THEN 'Gillette Stadium'
    WHEN 'M31' THEN 'Lincoln Financial Field'
    WHEN 'M32' THEN 'Levi''s Stadium'
    WHEN 'M33' THEN 'NRG Stadium'
    WHEN 'M34' THEN 'BMO Field'
    WHEN 'M35' THEN 'Arrowhead Stadium'
    WHEN 'M36' THEN 'Estadio BBVA'
    WHEN 'M37' THEN 'Mercedes-Benz Stadium'
    WHEN 'M38' THEN 'SoFi Stadium'
    WHEN 'M39' THEN 'Hard Rock Stadium'
    WHEN 'M40' THEN 'BC Place'
    WHEN 'M41' THEN 'AT&T Stadium'
    WHEN 'M42' THEN 'Lincoln Financial Field'
    WHEN 'M43' THEN 'MetLife Stadium'
    WHEN 'M44' THEN 'Levi''s Stadium'
    WHEN 'M45' THEN 'NRG Stadium'
    WHEN 'M46' THEN 'Gillette Stadium'
    WHEN 'M47' THEN 'BMO Field'
    WHEN 'M48' THEN 'Estadio Akron'
    WHEN 'M49' THEN 'BC Place'
    WHEN 'M50' THEN 'Lumen Field'
    WHEN 'M51' THEN 'Hard Rock Stadium'
    WHEN 'M52' THEN 'Mercedes-Benz Stadium'
    WHEN 'M53' THEN 'Estadio Azteca'
    WHEN 'M54' THEN 'Estadio Akron'
    WHEN 'M55' THEN 'MetLife Stadium'
    WHEN 'M56' THEN 'Lincoln Financial Field'
    WHEN 'M57' THEN 'AT&T Stadium'
    WHEN 'M58' THEN 'Arrowhead Stadium'
    WHEN 'M59' THEN 'SoFi Stadium'
    WHEN 'M60' THEN 'Levi''s Stadium'
    WHEN 'M61' THEN 'Gillette Stadium'
    WHEN 'M62' THEN 'BMO Field'
    WHEN 'M63' THEN 'NRG Stadium'
    WHEN 'M64' THEN 'Estadio Akron'
    WHEN 'M65' THEN 'Lumen Field'
    WHEN 'M66' THEN 'BC Place'
    WHEN 'M67' THEN 'Arrowhead Stadium'
    WHEN 'M68' THEN 'AT&T Stadium'
    WHEN 'M69' THEN 'Hard Rock Stadium'
    WHEN 'M70' THEN 'Mercedes-Benz Stadium'
    WHEN 'M71' THEN 'MetLife Stadium'
    WHEN 'M72' THEN 'Lincoln Financial Field'
    -- Knockout (M73–M104) ──────────────────────────────────────
    WHEN 'M73'  THEN 'SoFi Stadium'
    WHEN 'M74'  THEN 'Gillette Stadium'
    WHEN 'M75'  THEN 'Estadio BBVA'
    WHEN 'M76'  THEN 'NRG Stadium'
    WHEN 'M77'  THEN 'MetLife Stadium'
    WHEN 'M78'  THEN 'AT&T Stadium'
    WHEN 'M79'  THEN 'Estadio Azteca'
    WHEN 'M80'  THEN 'Mercedes-Benz Stadium'
    WHEN 'M81'  THEN 'Levi''s Stadium'
    WHEN 'M82'  THEN 'Lumen Field'
    WHEN 'M83'  THEN 'BMO Field'
    WHEN 'M84'  THEN 'SoFi Stadium'
    WHEN 'M85'  THEN 'BC Place'
    WHEN 'M86'  THEN 'Hard Rock Stadium'
    WHEN 'M87'  THEN 'Arrowhead Stadium'
    WHEN 'M88'  THEN 'AT&T Stadium'
    WHEN 'M89'  THEN 'Lincoln Financial Field'
    WHEN 'M90'  THEN 'NRG Stadium'
    WHEN 'M91'  THEN 'MetLife Stadium'
    WHEN 'M92'  THEN 'Estadio Azteca'
    WHEN 'M93'  THEN 'AT&T Stadium'
    WHEN 'M94'  THEN 'Lumen Field'
    WHEN 'M95'  THEN 'Mercedes-Benz Stadium'
    WHEN 'M96'  THEN 'BC Place'
    WHEN 'M97'  THEN 'Gillette Stadium'
    WHEN 'M98'  THEN 'SoFi Stadium'
    WHEN 'M99'  THEN 'Hard Rock Stadium'
    WHEN 'M100' THEN 'Arrowhead Stadium'
    WHEN 'M101' THEN 'AT&T Stadium'
    WHEN 'M102' THEN 'Mercedes-Benz Stadium'
    WHEN 'M103' THEN 'Hard Rock Stadium'
    WHEN 'M104' THEN 'MetLife Stadium'
    ELSE NULL
  END;
