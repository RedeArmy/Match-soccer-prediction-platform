-- Migration 000126: drop the kyb_profiles table.
--
-- The KYB (Know Your Business) module was removed from the Go service layer in
-- the kyc/aml branch after a scope decision to focus on individual KYC only.
-- Migration 000122 created this table but the corresponding repository, service,
-- and handler code was never shipped.  Keeping the dead schema creates confusion
-- and wastes space — dropping it now while the table is guaranteed to be empty.
--
-- Migrations are forward-only; no down migration is provided.
DROP TABLE IF EXISTS kyb_profiles CASCADE;
