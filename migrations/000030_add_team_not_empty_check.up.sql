-- Add check constraint to prevent storing an empty string in tournament_slots.team.
-- The service already validates this at the application level, but the constraint
-- closes the gap for direct database writes and is consistent with the pattern
-- used throughout the rest of the schema.
ALTER TABLE tournament_slots
    ADD CONSTRAINT chk_team_not_empty CHECK (team IS NULL OR team <> '');
