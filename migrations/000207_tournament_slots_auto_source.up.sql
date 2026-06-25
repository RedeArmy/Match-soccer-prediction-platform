-- Migration: auto_source column on tournament_slots + link home_slot_id/away_slot_id on knockout matches
--
-- auto_source stores the bracket placeholder code that identifies what a slot
-- represents. When a match finishes, the system looks up slots by auto_source
-- and confirms them automatically (no admin action required).
--
-- Codes follow the same convention as the knockout match seeds:
--   1A–1L   = 1st-place team in that group
--   2A–2L   = 2nd-place team in that group
--   W73–W103 = winner of the match with that FIFA match code
--   L101, L102 = loser of the semi-final match with that code
--
-- Best third-place slots (r32_13_a … r32_16_b) are intentionally left with
-- auto_source NULL: the bracket placement for 3rd-place qualifiers depends on
-- cross-group ranking rules applied after ALL groups finish and is handled
-- manually by the admin.

ALTER TABLE tournament_slots
  ADD COLUMN IF NOT EXISTS auto_source VARCHAR(20);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tournament_slots_auto_source
  ON tournament_slots (auto_source)
  WHERE auto_source IS NOT NULL;

-- ── Group position slots ──────────────────────────────────────────────────────
-- Derive auto_source from description using regex: 'X.° Grupo Y' → 'XY'
UPDATE tournament_slots
SET auto_source = (regexp_match(description, '^(\d+)\.° Grupo ([A-L])$'))[1]
               || (regexp_match(description, '^(\d+)\.° Grupo ([A-L])$'))[2]
WHERE description ~ '^\d+\.° Grupo [A-L]$';

-- ── Round-of-16 entry slots (fed by R32 winners) ─────────────────────────────
-- Derived from M89–M96 seed home_team / away_team placeholder codes.
UPDATE tournament_slots SET auto_source = 'W74'  WHERE label = 'r16_01_a';
UPDATE tournament_slots SET auto_source = 'W77'  WHERE label = 'r16_01_b';
UPDATE tournament_slots SET auto_source = 'W73'  WHERE label = 'r16_02_a';
UPDATE tournament_slots SET auto_source = 'W75'  WHERE label = 'r16_02_b';
UPDATE tournament_slots SET auto_source = 'W76'  WHERE label = 'r16_03_a';
UPDATE tournament_slots SET auto_source = 'W78'  WHERE label = 'r16_03_b';
UPDATE tournament_slots SET auto_source = 'W79'  WHERE label = 'r16_04_a';
UPDATE tournament_slots SET auto_source = 'W80'  WHERE label = 'r16_04_b';
UPDATE tournament_slots SET auto_source = 'W83'  WHERE label = 'r16_05_a';
UPDATE tournament_slots SET auto_source = 'W84'  WHERE label = 'r16_05_b';
UPDATE tournament_slots SET auto_source = 'W81'  WHERE label = 'r16_06_a';
UPDATE tournament_slots SET auto_source = 'W82'  WHERE label = 'r16_06_b';
UPDATE tournament_slots SET auto_source = 'W86'  WHERE label = 'r16_07_a';
UPDATE tournament_slots SET auto_source = 'W88'  WHERE label = 'r16_07_b';
UPDATE tournament_slots SET auto_source = 'W85'  WHERE label = 'r16_08_a';
UPDATE tournament_slots SET auto_source = 'W87'  WHERE label = 'r16_08_b';

-- ── Quarter-final entry slots (fed by R16 winners) ───────────────────────────
UPDATE tournament_slots SET auto_source = 'W89'  WHERE label = 'qf_01_a';
UPDATE tournament_slots SET auto_source = 'W90'  WHERE label = 'qf_01_b';
UPDATE tournament_slots SET auto_source = 'W93'  WHERE label = 'qf_02_a';
UPDATE tournament_slots SET auto_source = 'W94'  WHERE label = 'qf_02_b';
UPDATE tournament_slots SET auto_source = 'W91'  WHERE label = 'qf_03_a';
UPDATE tournament_slots SET auto_source = 'W92'  WHERE label = 'qf_03_b';
UPDATE tournament_slots SET auto_source = 'W95'  WHERE label = 'qf_04_a';
UPDATE tournament_slots SET auto_source = 'W96'  WHERE label = 'qf_04_b';

-- ── Semi-final entry slots (fed by QF winners) ───────────────────────────────
UPDATE tournament_slots SET auto_source = 'W97'  WHERE label = 'sf_01_a';
UPDATE tournament_slots SET auto_source = 'W98'  WHERE label = 'sf_01_b';
UPDATE tournament_slots SET auto_source = 'W99'  WHERE label = 'sf_02_a';
UPDATE tournament_slots SET auto_source = 'W100' WHERE label = 'sf_02_b';

-- ── Third-place play-off slots (fed by SF losers) ────────────────────────────
UPDATE tournament_slots SET auto_source = 'L101' WHERE label = 'tp_01_a';
UPDATE tournament_slots SET auto_source = 'L102' WHERE label = 'tp_01_b';

-- ── Final slots (fed by SF winners) ─────────────────────────────────────────
UPDATE tournament_slots SET auto_source = 'W101' WHERE label = 'fin_01_a';
UPDATE tournament_slots SET auto_source = 'W102' WHERE label = 'fin_01_b';

-- ── Link home_slot_id / away_slot_id on knockout matches ─────────────────────
-- Matches whose home_team / away_team hold a placeholder code that now matches
-- a slot's auto_source are linked so that ConfirmSlot propagates team names.
UPDATE matches m
SET home_slot_id = (SELECT id FROM tournament_slots ts WHERE ts.auto_source = m.home_team),
    away_slot_id = (SELECT id FROM tournament_slots ts WHERE ts.auto_source = m.away_team)
WHERE m.phase <> 'group_stage'
  AND (m.home_slot_id IS NULL OR m.away_slot_id IS NULL)
  AND EXISTS (SELECT 1 FROM tournament_slots ts WHERE ts.auto_source = m.home_team)
  AND EXISTS (SELECT 1 FROM tournament_slots ts WHERE ts.auto_source = m.away_team);
