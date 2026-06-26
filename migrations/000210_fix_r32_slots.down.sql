-- Revert 000210: restore the original r32 slots from migrations 000205 + 000207.

-- ── Step 1: remove the corrected slots ───────────────────────────────────────
DELETE FROM tournament_slots WHERE label LIKE 'r32_%';

-- ── Step 2: restore the original slots (migration 000205 structure) ───────────
INSERT INTO tournament_slots (label, description, auto_source) VALUES
  ('r32_01_a', '1.° Grupo A',    '1A'),
  ('r32_01_b', '2.° Grupo B',    '2B'),
  ('r32_02_a', '1.° Grupo C',    '1C'),
  ('r32_02_b', '2.° Grupo D',    '2D'),
  ('r32_03_a', '1.° Grupo E',    '1E'),
  ('r32_03_b', '2.° Grupo F',    '2F'),
  ('r32_04_a', '1.° Grupo G',    '1G'),
  ('r32_04_b', '2.° Grupo H',    '2H'),
  ('r32_05_a', '1.° Grupo I',    '1I'),
  ('r32_05_b', '2.° Grupo J',    '2J'),
  ('r32_06_a', '1.° Grupo K',    '1K'),
  ('r32_06_b', '2.° Grupo L',    '2L'),
  ('r32_07_a', '2.° Grupo A',    '2A'),
  ('r32_07_b', '1.° Grupo B',    '1B'),
  ('r32_08_a', '2.° Grupo C',    '2C'),
  ('r32_08_b', '1.° Grupo D',    '1D'),
  ('r32_09_a', '2.° Grupo E',    '2E'),
  ('r32_09_b', '1.° Grupo F',    '1F'),
  ('r32_10_a', '2.° Grupo G',    '2G'),
  ('r32_10_b', '1.° Grupo H',    '1H'),
  ('r32_11_a', '2.° Grupo I',    '2I'),
  ('r32_11_b', '1.° Grupo J',    '1J'),
  ('r32_12_a', '2.° Grupo K',    '2K'),
  ('r32_12_b', '1.° Grupo L',    '1L'),
  ('r32_13_a', 'Mejor 3.° (p.1)', NULL),
  ('r32_13_b', 'Mejor 3.° (p.2)', NULL),
  ('r32_14_a', 'Mejor 3.° (p.3)', NULL),
  ('r32_14_b', 'Mejor 3.° (p.4)', NULL),
  ('r32_15_a', 'Mejor 3.° (p.5)', NULL),
  ('r32_15_b', 'Mejor 3.° (p.6)', NULL),
  ('r32_16_a', 'Mejor 3.° (p.7)', NULL),
  ('r32_16_b', 'Mejor 3.° (p.8)', NULL);

-- ── Step 3: re-link matches whose both sides have an auto_source slot ─────────
-- Replicates the logic from migration 000207 for r32 matches.
UPDATE matches m
SET home_slot_id = (SELECT id FROM tournament_slots ts WHERE ts.auto_source = m.home_team),
    away_slot_id = (SELECT id FROM tournament_slots ts WHERE ts.auto_source = m.away_team)
WHERE m.phase = 'round_of_32'
  AND EXISTS (SELECT 1 FROM tournament_slots ts WHERE ts.auto_source = m.home_team)
  AND EXISTS (SELECT 1 FROM tournament_slots ts WHERE ts.auto_source = m.away_team);
