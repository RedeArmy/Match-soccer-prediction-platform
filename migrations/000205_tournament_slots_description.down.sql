-- Remove the 64 pre-seeded bracket slots and drop the description column.
-- WARNING: this deletes any slots that were inserted by the up migration.
-- Slots that were later confirmed (team IS NOT NULL) will also be removed.

DELETE FROM tournament_slots WHERE label IN (
  -- Round of 32
  'r32_01_a','r32_01_b','r32_02_a','r32_02_b',
  'r32_03_a','r32_03_b','r32_04_a','r32_04_b',
  'r32_05_a','r32_05_b','r32_06_a','r32_06_b',
  'r32_07_a','r32_07_b','r32_08_a','r32_08_b',
  'r32_09_a','r32_09_b','r32_10_a','r32_10_b',
  'r32_11_a','r32_11_b','r32_12_a','r32_12_b',
  'r32_13_a','r32_13_b','r32_14_a','r32_14_b',
  'r32_15_a','r32_15_b','r32_16_a','r32_16_b',
  -- Round of 16
  'r16_01_a','r16_01_b','r16_02_a','r16_02_b',
  'r16_03_a','r16_03_b','r16_04_a','r16_04_b',
  'r16_05_a','r16_05_b','r16_06_a','r16_06_b',
  'r16_07_a','r16_07_b','r16_08_a','r16_08_b',
  -- Quarter-finals
  'qf_01_a','qf_01_b','qf_02_a','qf_02_b',
  'qf_03_a','qf_03_b','qf_04_a','qf_04_b',
  -- Semi-finals
  'sf_01_a','sf_01_b','sf_02_a','sf_02_b',
  -- Third place
  'tp_01_a','tp_01_b',
  -- Final
  'fin_01_a','fin_01_b'
);

ALTER TABLE tournament_slots DROP COLUMN IF EXISTS description;
