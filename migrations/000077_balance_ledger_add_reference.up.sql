-- Adds an external reference key to balance_ledger for webhook idempotency.
--
-- The reference column stores the provider-issued transaction identifier
-- (e.g. a Recurrente transaction ID or a PayPal capture ID).  When a webhook
-- is re-delivered, the INSERT in CreditIdempotent hits ON CONFLICT DO NOTHING
-- and the balance update is skipped, preventing double-credits.
--
-- The partial index covers only non-NULL values so that ledger rows for
-- operations without an external reference (entry fees, prizes, withdrawals)
-- are not affected by the constraint.

ALTER TABLE balance_ledger ADD COLUMN reference TEXT;

CREATE UNIQUE INDEX balance_ledger_reference_uniq
    ON balance_ledger (reference)
    WHERE reference IS NOT NULL;
