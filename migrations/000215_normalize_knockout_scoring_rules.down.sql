-- Revert knockout scoring rules to the original escalating values.
UPDATE scoring_rules
SET    exact_score     = 6,
       correct_outcome = 3,
       goal_difference = 1,
       updated_at      = NOW()
WHERE  phase = 'round_of_32';

UPDATE scoring_rules
SET    exact_score     = 8,
       correct_outcome = 4,
       goal_difference = 2,
       updated_at      = NOW()
WHERE  phase = 'round_of_16';

UPDATE scoring_rules
SET    exact_score     = 10,
       correct_outcome = 5,
       goal_difference = 2,
       updated_at      = NOW()
WHERE  phase = 'quarter_final';

UPDATE scoring_rules
SET    exact_score     = 12,
       correct_outcome = 6,
       goal_difference = 3,
       updated_at      = NOW()
WHERE  phase IN ('semi_final', 'third_place');

UPDATE scoring_rules
SET    exact_score     = 15,
       correct_outcome = 8,
       goal_difference = 3,
       updated_at      = NOW()
WHERE  phase = 'final';
