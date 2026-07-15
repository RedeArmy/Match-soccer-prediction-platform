-- Adds two new match extras: which period (first half, second half, both,
-- or none) each team scores in. Both are fully derivable from data already
-- captured on matches (halftime_home_score/halftime_away_score plus the
-- final home_score/away_score) — no new sync/provider work is required.
--
-- Both extra_rules and extra_predictions CHECK extra_type against a fixed
-- allow-list (migrations 000225/000226); widen both before inserting rows
-- with the two new values.
ALTER TABLE extra_rules DROP CONSTRAINT extra_rules_extra_type_check;
ALTER TABLE extra_rules ADD CONSTRAINT extra_rules_extra_type_check
    CHECK (extra_type IN ('first_scorer', 'halftime_result', 'home_team_scores', 'away_team_scores'));

ALTER TABLE extra_predictions DROP CONSTRAINT extra_predictions_extra_type_check;
ALTER TABLE extra_predictions ADD CONSTRAINT extra_predictions_extra_type_check
    CHECK (extra_type IN ('first_scorer', 'halftime_result', 'home_team_scores', 'away_team_scores'));

-- Seeded at the same default point value: a symmetric 4-way guess per team,
-- comparable in difficulty to first_scorer.
INSERT INTO extra_rules (extra_type, points) VALUES
    ('home_team_scores', 3),
    ('away_team_scores', 3);

-- The half-time result extra changed from a 3-way outcome guess (home/draw/
-- away) to an exact half-time scoreline (e.g. "1-0"), matching the format
-- of the main prediction's final-score input. It is a strictly harder guess
-- than the outcome-only version, so its default point value is raised
-- accordingly (existing rows keep any admin-set override; this only updates
-- rows still at the old default of 2).
UPDATE extra_rules SET points = 4 WHERE extra_type = 'halftime_result' AND points = 2;
