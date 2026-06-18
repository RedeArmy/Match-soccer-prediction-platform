DROP INDEX IF EXISTS payment_intents_under_review_idx;

ALTER TABLE payment_intents
  DROP COLUMN IF EXISTS user_notes,
  DROP COLUMN IF EXISTS comprobante_required;

ALTER TABLE payment_intents
  DROP CONSTRAINT payment_intents_status_check;

ALTER TABLE payment_intents
  ADD CONSTRAINT payment_intents_status_check
    CHECK (status IN ('pending','captured','expired','rejected'));
