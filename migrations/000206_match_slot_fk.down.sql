ALTER TABLE matches
  DROP COLUMN IF EXISTS home_slot_id,
  DROP COLUMN IF EXISTS away_slot_id;
