-- Reverse of 000133: remove the stub profiles created by the backfill.
--
-- Only deletes rows that are still in the initial unverified/tier-0 state
-- with no KYC data submitted. Profiles that have been updated (e.g. a user
-- submitted an application after the backfill) are intentionally preserved.
--
-- WARNING: Do not run this down migration on a production database unless you
-- also revert the corresponding application code changes.

DELETE FROM kyc_profiles
WHERE status      = 'unverified'
  AND tier        = 0
  AND full_name   = ''
  AND submitted_at IS NULL;
