-- Migration 000197: Add cancelled status to payment_intents.
--
-- cancelled: set when the user abandons the payment provider's checkout
-- (closes the Recurrente popup, clicks Cancel on PayPal, etc.) without
-- completing the payment. The intent is still recorded for audit purposes
-- but is excluded from user-facing transaction history.

ALTER TABLE payment_intents
  DROP CONSTRAINT payment_intents_status_check;

ALTER TABLE payment_intents
  ADD CONSTRAINT payment_intents_status_check
    CHECK (status IN ('pending','captured','expired','rejected','under_review','cancelled'));

CREATE INDEX payment_intents_cancelled_idx ON payment_intents(status)
  WHERE status = 'cancelled';
