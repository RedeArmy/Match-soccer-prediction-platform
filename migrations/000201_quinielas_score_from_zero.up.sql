ALTER TABLE quinielas
  ADD COLUMN score_from_zero       BOOLEAN    NOT NULL DEFAULT FALSE,
  ADD COLUMN score_from_zero_since TIMESTAMPTZ;
