-- Backfill confirmed slot team names to knockout matches that still carry
-- placeholder codes (e.g. "3ABCDF", "1E", "W73") in home_team / away_team.
--
-- Root cause: AutoConfirmBestThirdSlots (and BackfillSlots) confirm slots and
-- propagate team names at confirmation time only.  If migration 000210 ran
-- after slots were already confirmed it reset home_slot_id / away_slot_id to
-- NULL (ON DELETE SET NULL) before re-linking matches to new slots, leaving the
-- new confirmed slots with no subsequent propagation pass.  Re-running the
-- admin "Auto-Confirmar" button also does not help because the service skips
-- already-confirmed slots (alreadyDone guard).
--
-- This migration is fully idempotent: it only touches rows where the team
-- field still matches a bracket placeholder pattern and the linked slot is
-- already confirmed (team IS NOT NULL).

-- ── Step 1: repair any missing slot links via auto_source ─────────────────────
-- Slots with auto_source (all non-best-third slots) can be linked by matching
-- the placeholder code stored in home_team / away_team to the slot auto_source.
-- Best-third slots (auto_source NULL) are already linked by match_code in
-- migration 000210 and are not affected by this step.

UPDATE matches m
SET home_slot_id = ts.id,
    updated_at   = NOW()
FROM tournament_slots ts
WHERE ts.auto_source IS NOT NULL
  AND ts.auto_source = m.home_team
  AND m.phase        <> 'group_stage'
  AND m.home_slot_id IS NULL;

UPDATE matches m
SET away_slot_id = ts.id,
    updated_at   = NOW()
FROM tournament_slots ts
WHERE ts.auto_source IS NOT NULL
  AND ts.auto_source = m.away_team
  AND m.phase        <> 'group_stage'
  AND m.away_slot_id IS NULL;

-- ── Step 2: propagate confirmed team names to matches with placeholder home ───
-- Covers both definitive positions (1A, 2B …) and winner/loser slots (W73 …).
UPDATE matches m
SET home_team  = ts.team,
    updated_at = NOW()
FROM tournament_slots ts
WHERE m.home_slot_id = ts.id
  AND ts.team IS NOT NULL
  AND m.home_team ~ '^(\d+[A-L]+|[WL]\d+)$';

-- ── Step 3: propagate confirmed team names to matches with placeholder away ───
-- Covers best-third slots (3ABCDF …) and all other away placeholders.
UPDATE matches m
SET away_team  = ts.team,
    updated_at = NOW()
FROM tournament_slots ts
WHERE m.away_slot_id = ts.id
  AND ts.team IS NOT NULL
  AND m.away_team ~ '^(\d+[A-L]+|[WL]\d+)$';
