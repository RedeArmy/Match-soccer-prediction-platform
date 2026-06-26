-- Rebuild r32 tournament_slots to match the official FIFA 2026 bracket (M73–M88).
--
-- Migration 000205 used a simplified cross-group layout (1A vs 2B, 1C vs 2D…)
-- that does not reflect the actual bracket. Eight of the sixteen r32 matches
-- include a conditional "best-3rd-place" opponent (coded 3ABCDF, 3CDFGH…),
-- which means their slot was never linked, breaking score propagation and the
-- public bracket display.
--
-- This migration:
--   1. Deletes existing r32_* slots (matches FKs become NULL automatically).
--   2. Inserts 32 new slots in match-code order (r32_01 = M73 … r32_16 = M88).
--      Definitive group positions carry auto_source so BackfillSlots can
--      auto-confirm them; conditional "best 3rd" slots have auto_source = NULL
--      and require admin confirmation.
--   3. Links each r32 match to its two new slots by match_code.
--
-- Slot numbering (bracket match order, not date order):
--   r32_01  M73   2A     vs  2B
--   r32_02  M74   1E     vs  3ABCDF   (conditional best-3rd)
--   r32_03  M75   1F     vs  2C
--   r32_04  M76   1C     vs  2F
--   r32_05  M77   1I     vs  3CDFGH   (conditional best-3rd)
--   r32_06  M78   2E     vs  2I
--   r32_07  M79   1A     vs  3CEFHI   (conditional best-3rd)
--   r32_08  M80   1L     vs  3EHIJK   (conditional best-3rd)
--   r32_09  M81   1D     vs  3BEFIJ   (conditional best-3rd)
--   r32_10  M82   1G     vs  3AEHIJ   (conditional best-3rd)
--   r32_11  M83   2K     vs  2L
--   r32_12  M84   1H     vs  2J
--   r32_13  M85   1B     vs  3EFGIJ   (conditional best-3rd)
--   r32_14  M86   1J     vs  2H
--   r32_15  M87   1K     vs  3DEIJL   (conditional best-3rd)
--   r32_16  M88   2D     vs  2G

-- ── Step 1: remove stale slots ───────────────────────────────────────────────
-- home_slot_id / away_slot_id on matches cascade to NULL (FK ON DELETE SET NULL).
DELETE FROM tournament_slots WHERE label LIKE 'r32_%';

-- ── Step 2: insert correctly-aligned slots ───────────────────────────────────
INSERT INTO tournament_slots (label, description, auto_source) VALUES
  -- M73: 2A vs 2B
  ('r32_01_a', '2.° Grupo A',             '2A'),
  ('r32_01_b', '2.° Grupo B',             '2B'),
  -- M74: 1E vs best-3rd(A/B/C/D/F)
  ('r32_02_a', '1.° Grupo E',             '1E'),
  ('r32_02_b', 'Mejor 3.° (A/B/C/D/F)',  NULL),
  -- M75: 1F vs 2C
  ('r32_03_a', '1.° Grupo F',             '1F'),
  ('r32_03_b', '2.° Grupo C',             '2C'),
  -- M76: 1C vs 2F
  ('r32_04_a', '1.° Grupo C',             '1C'),
  ('r32_04_b', '2.° Grupo F',             '2F'),
  -- M77: 1I vs best-3rd(C/D/F/G/H)
  ('r32_05_a', '1.° Grupo I',             '1I'),
  ('r32_05_b', 'Mejor 3.° (C/D/F/G/H)',  NULL),
  -- M78: 2E vs 2I
  ('r32_06_a', '2.° Grupo E',             '2E'),
  ('r32_06_b', '2.° Grupo I',             '2I'),
  -- M79: 1A vs best-3rd(C/E/F/H/I)
  ('r32_07_a', '1.° Grupo A',             '1A'),
  ('r32_07_b', 'Mejor 3.° (C/E/F/H/I)',  NULL),
  -- M80: 1L vs best-3rd(E/H/I/J/K)
  ('r32_08_a', '1.° Grupo L',             '1L'),
  ('r32_08_b', 'Mejor 3.° (E/H/I/J/K)',  NULL),
  -- M81: 1D vs best-3rd(B/E/F/I/J)
  ('r32_09_a', '1.° Grupo D',             '1D'),
  ('r32_09_b', 'Mejor 3.° (B/E/F/I/J)',  NULL),
  -- M82: 1G vs best-3rd(A/E/H/I/J)
  ('r32_10_a', '1.° Grupo G',             '1G'),
  ('r32_10_b', 'Mejor 3.° (A/E/H/I/J)',  NULL),
  -- M83: 2K vs 2L
  ('r32_11_a', '2.° Grupo K',             '2K'),
  ('r32_11_b', '2.° Grupo L',             '2L'),
  -- M84: 1H vs 2J
  ('r32_12_a', '1.° Grupo H',             '1H'),
  ('r32_12_b', '2.° Grupo J',             '2J'),
  -- M85: 1B vs best-3rd(E/F/G/I/J)
  ('r32_13_a', '1.° Grupo B',             '1B'),
  ('r32_13_b', 'Mejor 3.° (E/F/G/I/J)',  NULL),
  -- M86: 1J vs 2H
  ('r32_14_a', '1.° Grupo J',             '1J'),
  ('r32_14_b', '2.° Grupo H',             '2H'),
  -- M87: 1K vs best-3rd(D/E/I/J/L)
  ('r32_15_a', '1.° Grupo K',             '1K'),
  ('r32_15_b', 'Mejor 3.° (D/E/I/J/L)',  NULL),
  -- M88: 2D vs 2G
  ('r32_16_a', '2.° Grupo D',             '2D'),
  ('r32_16_b', '2.° Grupo G',             '2G');

-- ── Step 3: link each r32 match to its pair of slots ────────────────────────
-- Using match_code for an unambiguous join.
UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_01_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_01_b')
WHERE match_code = 'M73';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_02_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_02_b')
WHERE match_code = 'M74';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_03_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_03_b')
WHERE match_code = 'M75';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_04_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_04_b')
WHERE match_code = 'M76';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_05_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_05_b')
WHERE match_code = 'M77';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_06_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_06_b')
WHERE match_code = 'M78';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_07_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_07_b')
WHERE match_code = 'M79';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_08_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_08_b')
WHERE match_code = 'M80';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_09_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_09_b')
WHERE match_code = 'M81';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_10_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_10_b')
WHERE match_code = 'M82';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_11_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_11_b')
WHERE match_code = 'M83';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_12_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_12_b')
WHERE match_code = 'M84';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_13_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_13_b')
WHERE match_code = 'M85';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_14_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_14_b')
WHERE match_code = 'M86';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_15_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_15_b')
WHERE match_code = 'M87';

UPDATE matches SET
  home_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_16_a'),
  away_slot_id = (SELECT id FROM tournament_slots WHERE label = 'r32_16_b')
WHERE match_code = 'M88';
