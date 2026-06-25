UPDATE matches SET home_slot_id = NULL, away_slot_id = NULL
WHERE phase <> 'group_stage';

DROP INDEX IF EXISTS uq_tournament_slots_auto_source;

ALTER TABLE tournament_slots DROP COLUMN IF EXISTS auto_source;
