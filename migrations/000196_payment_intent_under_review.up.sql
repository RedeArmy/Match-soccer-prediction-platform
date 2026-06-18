-- Migration 000196: Add under_review status, comprobante_required flag, and user_notes to payment_intents.
--
-- under_review: set when a rejected user submits a dispute with evidence.
-- comprobante_required: set by admin on a pending intent to request the user upload a receipt.
-- user_notes: user-provided description when disputing a rejection.

ALTER TABLE payment_intents
  DROP CONSTRAINT payment_intents_status_check;

ALTER TABLE payment_intents
  ADD CONSTRAINT payment_intents_status_check
    CHECK (status IN ('pending','captured','expired','rejected','under_review'));

ALTER TABLE payment_intents
  ADD COLUMN comprobante_required BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN user_notes           TEXT    NOT NULL DEFAULT '';

CREATE INDEX payment_intents_under_review_idx ON payment_intents(status)
  WHERE status = 'under_review';
