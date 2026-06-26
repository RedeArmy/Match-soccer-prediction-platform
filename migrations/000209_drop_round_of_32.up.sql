-- Remove round_of_32 from the tournament model.
-- FIFA WC 2026 has 32 teams in the knockout stage, but this quiniela
-- starts bracket predictions at round_of_16 (16 matches → 8 → 4 → 2 → Final).

-- ── Tournament slots ─────────────────────────────────────────────────────────
DELETE FROM tournament_slots WHERE label LIKE 'r32_%';

-- r16 slots were fed by r32 winners (auto_source W73–W88). Since r32 is
-- removed, auto_source must be cleared so the admin confirms r16 slots manually
-- once group-stage results are known.
UPDATE tournament_slots
SET    auto_source  = NULL,
       description  = regexp_replace(description, '^Ganador R32 ', 'Clasificado R16 ')
WHERE  label LIKE 'r16_%';

-- ── Matches ──────────────────────────────────────────────────────────────────
-- Cascade-deletes predictions and scoring logs for these fixtures.
DELETE FROM matches WHERE phase = 'round_of_32';

-- ── Scoring rules ────────────────────────────────────────────────────────────
DELETE FROM scoring_rules WHERE phase = 'round_of_32';

-- ── Phase CHECK constraints ───────────────────────────────────────────────────
ALTER TABLE matches
  DROP CONSTRAINT IF EXISTS matches_phase_check;
ALTER TABLE matches
  ADD CONSTRAINT matches_phase_check
  CHECK (phase = ANY (ARRAY[
    'group_stage'::text, 'round_of_16'::text, 'quarter_final'::text,
    'semi_final'::text,  'third_place'::text,  'final'::text
  ]));

ALTER TABLE scoring_rules
  DROP CONSTRAINT IF EXISTS scoring_rules_phase_check;
ALTER TABLE scoring_rules
  ADD CONSTRAINT scoring_rules_phase_check
  CHECK (phase = ANY (ARRAY[
    'group_stage'::text, 'round_of_16'::text, 'quarter_final'::text,
    'semi_final'::text,  'third_place'::text,  'final'::text
  ]));
