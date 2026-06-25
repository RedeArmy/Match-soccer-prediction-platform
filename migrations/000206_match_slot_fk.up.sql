ALTER TABLE matches
  ADD COLUMN IF NOT EXISTS home_slot_id INT REFERENCES tournament_slots(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS away_slot_id INT REFERENCES tournament_slots(id) ON DELETE SET NULL;
