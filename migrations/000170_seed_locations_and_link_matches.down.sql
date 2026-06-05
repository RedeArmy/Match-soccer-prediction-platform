-- Reverses 000170: clears stadium_id on all seeded matches, then removes
-- stadiums, cities, states, and countries inserted by this migration.
-- Only rows seeded in 000170 are removed (ON CONFLICT DO NOTHING means
-- pre-existing rows from 000015 are left untouched).

-- 1. Unlink matches from stadiums
UPDATE matches SET stadium_id = NULL
WHERE match_code IN (
    'M1',  'M2',  'M3',  'M4',  'M5',  'M6',  'M7',  'M8',
    'M9',  'M10', 'M11', 'M12', 'M13', 'M14', 'M15', 'M16',
    'M17', 'M18', 'M19', 'M20', 'M21', 'M22', 'M23', 'M24',
    'M25', 'M26', 'M27', 'M28', 'M29', 'M30', 'M31', 'M32',
    'M33', 'M34', 'M35', 'M36', 'M37', 'M38', 'M39', 'M40',
    'M41', 'M42', 'M43', 'M44', 'M45', 'M46', 'M47', 'M48',
    'M49', 'M50', 'M51', 'M52', 'M53', 'M54', 'M55', 'M56',
    'M57', 'M58', 'M59', 'M60', 'M61', 'M62', 'M63', 'M64',
    'M65', 'M66', 'M67', 'M68', 'M69', 'M70', 'M71', 'M72',
    'M73',  'M74',  'M75',  'M76',  'M77',  'M78',  'M79',  'M80',
    'M81',  'M82',  'M83',  'M84',  'M85',  'M86',  'M87',  'M88',
    'M89',  'M90',  'M91',  'M92',  'M93',  'M94',  'M95',  'M96',
    'M97',  'M98',  'M99',  'M100', 'M101', 'M102', 'M103', 'M104'
);

-- 2. Remove stadiums seeded in this migration
DELETE FROM stadiums WHERE name IN (
    'MetLife Stadium', 'AT&T Stadium', 'SoFi Stadium', 'Hard Rock Stadium',
    'Levi''s Stadium', 'Lincoln Financial Field', 'Lumen Field', 'Arrowhead Stadium',
    'Mercedes-Benz Stadium', 'NRG Stadium', 'Gillette Stadium',
    'Estadio Azteca', 'Estadio BBVA', 'Estadio Akron',
    'BC Place', 'BMO Field'
);

-- 3. Remove cities seeded in this migration
DELETE FROM cities WHERE name IN (
    'East Rutherford', 'Arlington', 'Inglewood', 'Miami Gardens', 'Santa Clara',
    'Philadelphia', 'Seattle', 'Kansas City', 'Atlanta', 'Houston', 'Foxborough',
    'Mexico City', 'Monterrey', 'Guadalajara', 'Vancouver', 'Toronto'
);

-- 4. Remove states seeded in this migration
DELETE FROM states WHERE (code, country_id) IN (
    SELECT s.code, s.country_id FROM states s
    JOIN countries c ON s.country_id = c.id
    WHERE (s.code, c.code::text) IN (
        ('NJ','US'),('TX','US'),('CA','US'),('FL','US'),('PA','US'),
        ('WA','US'),('MO','US'),('GA','US'),('MA','US'),
        ('CDMX','MX'),('NL','MX'),('JAL','MX'),
        ('BC','CA'),('ON','CA')
    )
);

-- 5. Remove countries seeded in this migration
DELETE FROM countries WHERE code IN ('US', 'MX', 'CA');
