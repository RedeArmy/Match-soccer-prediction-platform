-- Add description column to tournament_slots and pre-seed all 64 bracket positions.
--
-- Label convention: {phase}_{match:02d}_{side}
--   phase  = r32 | r16 | qf | sf | tp | fin
--   match  = zero-padded match number within the phase
--   side   = a (home/left) | b (away/right)
--
-- Description holds a Spanish human-readable placeholder shown until an
-- admin calls PATCH /tournament/slots/:id to confirm the advancing team.

ALTER TABLE tournament_slots ADD COLUMN IF NOT EXISTS description VARCHAR(120);

-- ── Round of 32 — Matches 01–06: group winners vs runners-up ─────────────────
INSERT INTO tournament_slots (label, description) VALUES
  ('r32_01_a', '1.° Grupo A'),
  ('r32_01_b', '2.° Grupo B'),
  ('r32_02_a', '1.° Grupo C'),
  ('r32_02_b', '2.° Grupo D'),
  ('r32_03_a', '1.° Grupo E'),
  ('r32_03_b', '2.° Grupo F'),
  ('r32_04_a', '1.° Grupo G'),
  ('r32_04_b', '2.° Grupo H'),
  ('r32_05_a', '1.° Grupo I'),
  ('r32_05_b', '2.° Grupo J'),
  ('r32_06_a', '1.° Grupo K'),
  ('r32_06_b', '2.° Grupo L'),

-- ── Round of 32 — Matches 07–12: runners-up vs group winners ─────────────────
  ('r32_07_a', '2.° Grupo A'),
  ('r32_07_b', '1.° Grupo B'),
  ('r32_08_a', '2.° Grupo C'),
  ('r32_08_b', '1.° Grupo D'),
  ('r32_09_a', '2.° Grupo E'),
  ('r32_09_b', '1.° Grupo F'),
  ('r32_10_a', '2.° Grupo G'),
  ('r32_10_b', '1.° Grupo H'),
  ('r32_11_a', '2.° Grupo I'),
  ('r32_11_b', '1.° Grupo J'),
  ('r32_12_a', '2.° Grupo K'),
  ('r32_12_b', '1.° Grupo L'),

-- ── Round of 32 — Matches 13–16: best eight 3rd-place teams ──────────────────
  ('r32_13_a', 'Mejor 3.° (p.1)'),
  ('r32_13_b', 'Mejor 3.° (p.2)'),
  ('r32_14_a', 'Mejor 3.° (p.3)'),
  ('r32_14_b', 'Mejor 3.° (p.4)'),
  ('r32_15_a', 'Mejor 3.° (p.5)'),
  ('r32_15_b', 'Mejor 3.° (p.6)'),
  ('r32_16_a', 'Mejor 3.° (p.7)'),
  ('r32_16_b', 'Mejor 3.° (p.8)'),

-- ── Round of 16 ───────────────────────────────────────────────────────────────
  ('r16_01_a', 'Ganador R32 M01'),
  ('r16_01_b', 'Ganador R32 M02'),
  ('r16_02_a', 'Ganador R32 M03'),
  ('r16_02_b', 'Ganador R32 M04'),
  ('r16_03_a', 'Ganador R32 M05'),
  ('r16_03_b', 'Ganador R32 M06'),
  ('r16_04_a', 'Ganador R32 M07'),
  ('r16_04_b', 'Ganador R32 M08'),
  ('r16_05_a', 'Ganador R32 M09'),
  ('r16_05_b', 'Ganador R32 M10'),
  ('r16_06_a', 'Ganador R32 M11'),
  ('r16_06_b', 'Ganador R32 M12'),
  ('r16_07_a', 'Ganador R32 M13'),
  ('r16_07_b', 'Ganador R32 M14'),
  ('r16_08_a', 'Ganador R32 M15'),
  ('r16_08_b', 'Ganador R32 M16'),

-- ── Quarter-finals ────────────────────────────────────────────────────────────
  ('qf_01_a', 'Ganador R16 M01'),
  ('qf_01_b', 'Ganador R16 M02'),
  ('qf_02_a', 'Ganador R16 M03'),
  ('qf_02_b', 'Ganador R16 M04'),
  ('qf_03_a', 'Ganador R16 M05'),
  ('qf_03_b', 'Ganador R16 M06'),
  ('qf_04_a', 'Ganador R16 M07'),
  ('qf_04_b', 'Ganador R16 M08'),

-- ── Semi-finals ───────────────────────────────────────────────────────────────
  ('sf_01_a', 'Ganador QF M01'),
  ('sf_01_b', 'Ganador QF M02'),
  ('sf_02_a', 'Ganador QF M03'),
  ('sf_02_b', 'Ganador QF M04'),

-- ── Third-place play-off ──────────────────────────────────────────────────────
  ('tp_01_a', 'Perdedor SF M01'),
  ('tp_01_b', 'Perdedor SF M02'),

-- ── Final ─────────────────────────────────────────────────────────────────────
  ('fin_01_a', 'Ganador SF M01'),
  ('fin_01_b', 'Ganador SF M02')

ON CONFLICT (label) DO NOTHING;
