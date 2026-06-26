-- Restore round_of_32 CHECK constraints.
-- Data (matches, slots, scoring rule) is NOT restored — rerun 000169 and 000205.
ALTER TABLE matches
  DROP CONSTRAINT IF EXISTS matches_phase_check;
ALTER TABLE matches
  ADD CONSTRAINT matches_phase_check
  CHECK (phase = ANY (ARRAY[
    'group_stage'::text, 'round_of_32'::text, 'round_of_16'::text,
    'quarter_final'::text, 'semi_final'::text, 'third_place'::text, 'final'::text
  ]));

ALTER TABLE scoring_rules
  DROP CONSTRAINT IF EXISTS scoring_rules_phase_check;
ALTER TABLE scoring_rules
  ADD CONSTRAINT scoring_rules_phase_check
  CHECK (phase = ANY (ARRAY[
    'group_stage'::text, 'round_of_32'::text, 'round_of_16'::text,
    'quarter_final'::text, 'semi_final'::text, 'third_place'::text, 'final'::text
  ]));
