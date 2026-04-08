-- Migration: add stadium_id foreign key to matches
--
-- stadium_id is nullable: a match may be created before its venue is
-- confirmed (e.g. knockout-stage fixtures whose stadium is assigned later).
-- The ON DELETE SET NULL rule means removing a stadium row does not cascade
-- into deleting match rows — it simply clears the venue assignment.
ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS stadium_id INTEGER
        REFERENCES stadiums(id) ON DELETE SET NULL;
