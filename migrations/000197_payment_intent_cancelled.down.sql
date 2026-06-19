DROP INDEX IF EXISTS payment_intents_cancelled_idx;

ALTER TABLE payment_intents
  DROP CONSTRAINT payment_intents_status_check;

ALTER TABLE payment_intents
  ADD CONSTRAINT payment_intents_status_check
    CHECK (status IN ('pending','captured','expired','rejected','under_review'));
