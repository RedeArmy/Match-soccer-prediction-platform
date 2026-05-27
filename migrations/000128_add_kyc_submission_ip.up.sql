-- Add submission_ip to record the client IP address at the time of KYC submission.
-- Used by CheckIPSubmissionVelocity to rate-limit abuse from a single network.
-- INET type validates the address at insert time and enables PostgreSQL network operators.
ALTER TABLE kyc_profiles
    ADD COLUMN IF NOT EXISTS submission_ip INET;

-- Partial index covers the frequent velocity-check query: WHERE submission_ip = $1 AND submitted_at >= $2
CREATE INDEX IF NOT EXISTS idx_kyc_profiles_submission_ip_submitted_at
    ON kyc_profiles (submission_ip, submitted_at)
    WHERE submission_ip IS NOT NULL;
