-- Migration 000199: add source_currency and source_amount_cents to balance_ledger.
--
-- These columns preserve the original deposit currency and amount for ledger
-- entries generated from non-GTQ payments (e.g. USD via Recurrente). They are
-- NULL for GTQ-denominated credits and all existing rows.
--
-- source_currency    — ISO 4217 code of the original payment currency (e.g. "USD")
-- source_amount_cents — amount in source_currency minor units (e.g. 500 = $5.00)
--
-- The GTQ amount credited to balance_cents (delta_cents) is computed by the
-- service layer using the buy rate at the time of the webhook. Storing the
-- original allows the frontend to display the exact deposited amount without
-- rounding from a GTQ→USD back-conversion.

ALTER TABLE balance_ledger
  ADD COLUMN source_currency     VARCHAR(3),
  ADD COLUMN source_amount_cents INT;
