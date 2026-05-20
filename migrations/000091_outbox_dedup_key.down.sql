DROP INDEX IF EXISTS idx_outbox_dedup;
ALTER TABLE domain_outbox DROP COLUMN IF EXISTS dedup_key;
