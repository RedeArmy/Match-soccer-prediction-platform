-- Add group_label to matches for group-stage tracking.
-- Nullable: only group_stage matches carry a label ("A"–"L").
-- Knockout matches leave this NULL.
ALTER TABLE matches ADD COLUMN group_label VARCHAR(5);
