UPDATE extra_rules SET points = 2 WHERE extra_type = 'halftime_result' AND points = 4;
DELETE FROM extra_rules WHERE extra_type IN ('home_team_scores', 'away_team_scores');

ALTER TABLE extra_predictions DROP CONSTRAINT extra_predictions_extra_type_check;
ALTER TABLE extra_predictions ADD CONSTRAINT extra_predictions_extra_type_check
    CHECK (extra_type IN ('first_scorer', 'halftime_result'));

ALTER TABLE extra_rules DROP CONSTRAINT extra_rules_extra_type_check;
ALTER TABLE extra_rules ADD CONSTRAINT extra_rules_extra_type_check
    CHECK (extra_type IN ('first_scorer', 'halftime_result'));
