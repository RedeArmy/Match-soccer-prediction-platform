-- Normalise knockout scoring rules so that correct_outcome and goal_difference
-- are constant across all phases (matching the group-stage values of 2 and 1).
-- Only exact_score escalates to maintain the desired per-phase point ceiling:
--
--   round_of_32   →  max  8 (exact 6  + pen bonus 2)
--   round_of_16   →  max 10 (exact 8  + pen bonus 2)
--   quarter_final →  max 12 (exact 10 + pen bonus 2)
--   semi_final    →  max 14 (exact 12 + pen bonus 2)
--   third_place   →  max 14 (exact 12 + pen bonus 2)
--   final         →  max 16 (exact 14 + pen bonus 2)
--
-- Win-method bonuses (extra_time_bonus=1, penalties_bonus=2) are unchanged.
-- Predictions already scored under the old rules are protected by the
-- prediction_score_log snapshot: their point values are locked to the config
-- active at scoring time and will not change unless an admin forces a rescore.

UPDATE scoring_rules
SET    correct_outcome = 2,
       goal_difference = 1,
       updated_at      = NOW()
WHERE  phase IN ('round_of_32', 'round_of_16', 'quarter_final',
                 'semi_final', 'third_place', 'final');

-- Final exact_score drops from 15 → 14 to hit the max-16 ceiling.
UPDATE scoring_rules
SET    exact_score = 14,
       updated_at  = NOW()
WHERE  phase = 'final';
