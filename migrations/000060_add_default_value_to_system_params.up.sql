-- Add default_value column to preserve the original migration-seeded value
-- independently of any operator overrides applied via the admin API.
--
-- Semantics:
--   default_value  – set once at INSERT time (migration seed); never touched by Set/BulkSet.
--   value          – current operational value; may be overridden by admins at runtime.
--
-- This makes the migration default permanently recoverable (via PATCH /reset)
-- without re-running migrations or redeploying the application.

ALTER TABLE system_params ADD COLUMN IF NOT EXISTS default_value TEXT;

-- Backfill: treat every existing operational value as its own default.
-- From this point forward, new seeds must supply default_value explicitly.
UPDATE system_params SET default_value = value WHERE default_value IS NULL;

ALTER TABLE system_params ALTER COLUMN default_value SET NOT NULL;
