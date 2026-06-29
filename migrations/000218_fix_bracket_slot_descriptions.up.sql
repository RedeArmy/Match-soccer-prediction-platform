-- Fix slot descriptions for round-of-16 and quarter-final slots.
--
-- Migration 000205 seeded descriptions assuming slots would be filled in
-- sequential bracket order (r16_01_a ← R32 M01, r16_01_b ← R32 M02 …).
-- Migration 000207 later set auto_source values to reflect the actual FIFA
-- 2026 bracket draw, where the source brackets are NOT sequential.
-- This migration corrects the descriptions to match the auto_source routing.
--
-- The auto_source is the source of truth for team propagation; descriptions
-- are purely display text shown as placeholders before teams advance.

-- ── Round-of-16 slot descriptions ─────────────────────────────────────────────
-- auto_source → FIFA match code → r32 bracket number
-- r16_01_a: W74 → M74 → r32_02
UPDATE tournament_slots SET description = 'Ganador R32 M02' WHERE label = 'r16_01_a';
-- r16_01_b: W77 → M77 → r32_05
UPDATE tournament_slots SET description = 'Ganador R32 M05' WHERE label = 'r16_01_b';
-- r16_02_a: W73 → M73 → r32_01
UPDATE tournament_slots SET description = 'Ganador R32 M01' WHERE label = 'r16_02_a';
-- r16_02_b: W75 → M75 → r32_03
UPDATE tournament_slots SET description = 'Ganador R32 M03' WHERE label = 'r16_02_b';
-- r16_03_a: W76 → M76 → r32_04
UPDATE tournament_slots SET description = 'Ganador R32 M04' WHERE label = 'r16_03_a';
-- r16_03_b: W78 → M78 → r32_06 (already correct)
-- r16_04_a: W79 → M79 → r32_07 (already correct)
-- r16_04_b: W80 → M80 → r32_08 (already correct)
-- r16_05_a: W83 → M83 → r32_11
UPDATE tournament_slots SET description = 'Ganador R32 M11' WHERE label = 'r16_05_a';
-- r16_05_b: W84 → M84 → r32_12
UPDATE tournament_slots SET description = 'Ganador R32 M12' WHERE label = 'r16_05_b';
-- r16_06_a: W81 → M81 → r32_09
UPDATE tournament_slots SET description = 'Ganador R32 M09' WHERE label = 'r16_06_a';
-- r16_06_b: W82 → M82 → r32_10
UPDATE tournament_slots SET description = 'Ganador R32 M10' WHERE label = 'r16_06_b';
-- r16_07_a: W86 → M86 → r32_14
UPDATE tournament_slots SET description = 'Ganador R32 M14' WHERE label = 'r16_07_a';
-- r16_07_b: W88 → M88 → r32_16
UPDATE tournament_slots SET description = 'Ganador R32 M16' WHERE label = 'r16_07_b';
-- r16_08_a: W85 → M85 → r32_13
UPDATE tournament_slots SET description = 'Ganador R32 M13' WHERE label = 'r16_08_a';
-- r16_08_b: W87 → M87 → r32_15
UPDATE tournament_slots SET description = 'Ganador R32 M15' WHERE label = 'r16_08_b';

-- ── Quarter-final slot descriptions ───────────────────────────────────────────
-- qf_02 and qf_03 had their R16 source numbers swapped.
-- qf_02_a: W93 → M93 → r16_05 (was "M03")
UPDATE tournament_slots SET description = 'Ganador R16 M05' WHERE label = 'qf_02_a';
-- qf_02_b: W94 → M94 → r16_06 (was "M04")
UPDATE tournament_slots SET description = 'Ganador R16 M06' WHERE label = 'qf_02_b';
-- qf_03_a: W91 → M91 → r16_03 (was "M05")
UPDATE tournament_slots SET description = 'Ganador R16 M03' WHERE label = 'qf_03_a';
-- qf_03_b: W92 → M92 → r16_04 (was "M06")
UPDATE tournament_slots SET description = 'Ganador R16 M04' WHERE label = 'qf_03_b';
