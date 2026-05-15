DROP INDEX IF EXISTS balance_ledger_reference_uniq;
ALTER TABLE balance_ledger DROP COLUMN IF EXISTS reference;
