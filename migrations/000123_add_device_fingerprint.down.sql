DROP INDEX IF EXISTS idx_kyc_profiles_device_fingerprint;
ALTER TABLE kyc_profiles DROP COLUMN IF EXISTS device_fingerprint;
