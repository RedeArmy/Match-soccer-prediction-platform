-- Seed: FIFA World Cup 2026 knockout fixtures (M73–M104, 32 matches)
--
-- Sources:
--   Wikipedia "2026 FIFA World Cup knockout stage"
--   NBC Sports "2026 World Cup schedule confirmed dates times stadiums"
--   Atlanta FWC26 official match schedule (atlantafwc26.com/match-schedule)
--   CBS Sports "World Cup 2026 schedule times dates"
--
-- All kickoff times are UTC.  Bracket slot notation:
--   1X  = winner of Group X        2X  = runner-up of Group X
--   3XY = best third-place from groups X and Y (pool varies per slot)
--   WN  = winner of match N        LN  = loser of match N
--
-- home_team / away_team hold placeholder bracket descriptors; they will be
-- replaced by actual team names once the administrator confirms each slot
-- via the TournamentSlots system (POST /admin/tournament-slots/{id}/confirm).
--
-- group_label MUST be NULL for all knockout phases (schema constraint).
-- ON CONFLICT (home_team, away_team, kickoff_at) DO NOTHING ensures
-- idempotency on repeated runs.

INSERT INTO matches
    (home_team, away_team, status, phase, group_label, stadium_id, kickoff_at, match_code)
SELECT
    v.home_slot,
    v.away_slot,
    'scheduled',
    v.phase,
    NULL,   -- group_label must be NULL for all knockout matches
    (SELECT id FROM stadiums WHERE name = v.venue),
    v.ko::timestamptz,
    v.mc
FROM (VALUES
    -- ── Round of 32 (M73–M88) ───────────────────────────────────────────────
    -- Source: Wikipedia bracket + NBC Sports ET times (converted to UTC)
    ('2A',        '2B',        'round_of_32', 'SoFi Stadium',            '2026-06-28 19:00:00+00', 'M73'),
    ('1E',        '3ABCDF',    'round_of_32', 'Gillette Stadium',        '2026-06-29 20:30:00+00', 'M74'),
    ('1C',        '2F',        'round_of_32', 'NRG Stadium',             '2026-06-29 17:00:00+00', 'M76'),
    ('1F',        '2C',        'round_of_32', 'Estadio BBVA',            '2026-06-30 01:00:00+00', 'M75'),
    ('2E',        '2I',        'round_of_32', 'AT&T Stadium',            '2026-06-30 17:00:00+00', 'M78'),
    ('1I',        '3CDFGH',    'round_of_32', 'MetLife Stadium',         '2026-06-30 21:00:00+00', 'M77'),
    ('1A',        '3CEFHI',    'round_of_32', 'Estadio Azteca',          '2026-07-01 01:00:00+00', 'M79'),
    ('1L',        '3EHIJK',    'round_of_32', 'Mercedes-Benz Stadium',  '2026-07-01 16:00:00+00', 'M80'),
    ('1G',        '3AEHIJ',    'round_of_32', 'Lumen Field',             '2026-07-01 20:00:00+00', 'M82'),
    ('1D',        '3BEFIJ',    'round_of_32', 'Levi''s Stadium',         '2026-07-02 00:00:00+00', 'M81'),
    ('1H',        '2J',        'round_of_32', 'SoFi Stadium',            '2026-07-02 19:00:00+00', 'M84'),
    ('2K',        '2L',        'round_of_32', 'BMO Field',               '2026-07-02 23:00:00+00', 'M83'),
    ('1B',        '3EFGIJ',    'round_of_32', 'BC Place',                '2026-07-03 03:00:00+00', 'M85'),
    ('2D',        '2G',        'round_of_32', 'AT&T Stadium',            '2026-07-03 18:00:00+00', 'M88'),
    ('1J',        '2H',        'round_of_32', 'Hard Rock Stadium',       '2026-07-03 22:00:00+00', 'M86'),
    ('1K',        '3DEIJL',    'round_of_32', 'Arrowhead Stadium',       '2026-07-04 01:30:00+00', 'M87'),

    -- ── Round of 16 (M89–M96) ───────────────────────────────────────────────
    -- Bracket: W74 vs W77 → Philadelphia; W73 vs W75 → Houston; etc.
    -- Confirmed: M95 (W86 vs W88) at Atlanta Jul 7 12pm ET = 16:00 UTC
    ('W74',       'W77',       'round_of_16', 'Lincoln Financial Field', '2026-07-04 18:00:00+00', 'M89'),
    ('W73',       'W75',       'round_of_16', 'NRG Stadium',             '2026-07-04 21:00:00+00', 'M90'),
    ('W76',       'W78',       'round_of_16', 'MetLife Stadium',         '2026-07-05 20:00:00+00', 'M91'),
    ('W79',       'W80',       'round_of_16', 'Estadio Azteca',          '2026-07-05 23:00:00+00', 'M92'),
    ('W83',       'W84',       'round_of_16', 'AT&T Stadium',            '2026-07-06 19:00:00+00', 'M93'),
    ('W81',       'W82',       'round_of_16', 'Lumen Field',             '2026-07-06 22:00:00+00', 'M94'),
    ('W86',       'W88',       'round_of_16', 'Mercedes-Benz Stadium',  '2026-07-07 16:00:00+00', 'M95'),
    ('W85',       'W87',       'round_of_16', 'BC Place',                '2026-07-07 20:00:00+00', 'M96'),

    -- ── Quarterfinals (M97–M100) ─────────────────────────────────────────────
    ('W89',       'W90',       'quarter_final','Gillette Stadium',       '2026-07-09 19:00:00+00', 'M97'),
    ('W93',       'W94',       'quarter_final','SoFi Stadium',           '2026-07-10 22:00:00+00', 'M98'),
    ('W91',       'W92',       'quarter_final','Hard Rock Stadium',      '2026-07-11 19:00:00+00', 'M99'),
    ('W95',       'W96',       'quarter_final','Arrowhead Stadium',      '2026-07-11 22:00:00+00', 'M100'),

    -- ── Semifinals (M101–M102) ───────────────────────────────────────────────
    -- CBS Sports: both SFs at 3 pm ET; Atlanta confirmed Jul 15 3 pm ET
    ('W97',       'W98',       'semi_final',  'AT&T Stadium',            '2026-07-14 19:00:00+00', 'M101'),
    ('W99',       'W100',      'semi_final',  'Mercedes-Benz Stadium',  '2026-07-15 19:00:00+00', 'M102'),

    -- ── Third-place playoff (M103) ───────────────────────────────────────────
    -- CBS Sports: 5 pm ET Jul 18 = 21:00 UTC
    ('L101',      'L102',      'third_place', 'Hard Rock Stadium',       '2026-07-18 21:00:00+00', 'M103'),

    -- ── Final (M104) ─────────────────────────────────────────────────────────
    -- CBS Sports / FIFA: 3 pm ET Jul 19; MetLife Stadium, East Rutherford NJ
    ('W101',      'W102',      'final',       'MetLife Stadium',         '2026-07-19 19:00:00+00', 'M104')
) AS v(home_slot, away_slot, phase, venue, ko, mc)
ON CONFLICT (home_team, away_team, kickoff_at) DO NOTHING;
