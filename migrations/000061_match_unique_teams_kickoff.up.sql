-- Prevent duplicate match records (e.g. admin double-click or client retry).
-- Two matches between the same teams at the exact same kickoff time cannot
-- coexist; the service layer maps the resulting unique violation to a 409.
ALTER TABLE matches
    ADD CONSTRAINT uq_matches_teams_kickoff UNIQUE (home_team, away_team, kickoff_at);
