-- Seed: FIFA World Cup 2026 host stadiums (16 venues)
--
-- Sources: FIFA.com official venue list, Wikipedia "2026 FIFA World Cup venues"
-- Capacities are FIFA tournament capacities (may differ from regular seating).
--
-- city_id is resolved via the location hierarchy seeded in 000015; stadium
-- names are the official FIFA marketing names as of the tournament schedule.
-- ON CONFLICT (name) DO NOTHING ensures idempotency on repeated runs.

INSERT INTO stadiums (name, capacity, city_id)
SELECT v.name, v.capacity, ci.id
FROM (VALUES
    -- United States (11 venues) -----------------------------------------------
    -- Source: FIFA.com/en/tournaments/mens/worldcup/canadamexicousa2026/stadiums
    ('MetLife Stadium',       82500, 'East Rutherford', 'NJ',   'US'),
    ('AT&T Stadium',          80000, 'Arlington',       'TX',   'US'),
    ('SoFi Stadium',          70240, 'Inglewood',       'CA',   'US'),
    ('Hard Rock Stadium',     65326, 'Miami Gardens',   'FL',   'US'),
    ('Levi''s Stadium',       68500, 'Santa Clara',     'CA',   'US'),
    ('Lincoln Financial Field',69796,'Philadelphia',    'PA',   'US'),
    ('Lumen Field',           68740, 'Seattle',         'WA',   'US'),
    ('Arrowhead Stadium',     76416, 'Kansas City',     'MO',   'US'),
    ('Mercedes-Benz Stadium', 71000, 'Atlanta',         'GA',   'US'),
    ('NRG Stadium',           72220, 'Houston',         'TX',   'US'),
    ('Gillette Stadium',      65878, 'Foxborough',      'MA',   'US'),
    -- Mexico (3 venues) -------------------------------------------------------
    ('Estadio Azteca',        87523, 'Mexico City',     'CDMX', 'MX'),
    ('Estadio BBVA',          53500, 'Monterrey',       'NL',   'MX'),
    ('Estadio Akron',         49850, 'Guadalajara',     'JAL',  'MX'),
    -- Canada (2 venues) -------------------------------------------------------
    ('BC Place',              54500, 'Vancouver',       'BC',   'CA'),
    ('BMO Field',             45736, 'Toronto',         'ON',   'CA')
) AS v(name, capacity, city_name, state_code, country_code)
JOIN cities   ci ON ci.name         = v.city_name
JOIN states   st ON st.id           = ci.state_id  AND st.code        = v.state_code
JOIN countries co ON co.id          = st.country_id AND co.code::text  = v.country_code
ON CONFLICT (name) DO NOTHING;
