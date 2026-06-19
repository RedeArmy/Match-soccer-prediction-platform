ALTER TABLE balance_ledger
  DROP COLUMN IF EXISTS source_currency,
  DROP COLUMN IF EXISTS source_amount_cents;
