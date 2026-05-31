-- Migration 000146: add gtq_reserved_cents to withdrawal_requests.
--
-- balance_cents and reserved_cents are always denominated in GTQ centavos.
-- When a user requests a USD PayPal withdrawal the previous code reserved
-- amountCents (USD cents) from the GTQ balance — mixing currency units.
--
-- gtq_reserved_cents records the GTQ centavos actually held in reserved_cents
-- for each withdrawal, decoupling the user-facing requested amount
-- (amount_cents in the request's currency) from the internal balance operation.
--
-- Invariant:
--   GTQ withdrawals  → gtq_reserved_cents = amount_cents
--   USD withdrawals  → gtq_reserved_cents = amount_cents * usd_gtq_rate / 100
--
-- Backfill: all existing rows are assumed GTQ (USD PayPal flow was not yet
-- live at time of this migration), so gtq_reserved_cents = amount_cents.

ALTER TABLE withdrawal_requests ADD COLUMN gtq_reserved_cents INT;

UPDATE withdrawal_requests SET gtq_reserved_cents = amount_cents;

ALTER TABLE withdrawal_requests ALTER COLUMN gtq_reserved_cents SET NOT NULL;
