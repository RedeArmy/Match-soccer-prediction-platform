-- Migration 000133: backfill minimal kyc_profiles rows for existing users.
--
-- Every user must have a kyc_profiles row so that prize distribution can
-- always freeze a balance even before the user submits a KYC application.
-- Going forward, a row is created at registration via EnsureStub (called from
-- ClerkUserSyncService.Upsert). This migration covers all users already in the
-- database before that change was deployed.
--
-- The INSERT … ON CONFLICT DO NOTHING pattern is idempotent: re-running this
-- migration is safe and will not overwrite any data for users who have already
-- submitted a KYC application or been approved.

INSERT INTO kyc_profiles (user_id)
SELECT id FROM users
WHERE deleted_at IS NULL
ON CONFLICT (user_id) DO NOTHING;
