DROP INDEX IF EXISTS idx_kyc_profiles_submission_ip_submitted_at;
ALTER TABLE kyc_profiles DROP COLUMN IF EXISTS submission_ip;
