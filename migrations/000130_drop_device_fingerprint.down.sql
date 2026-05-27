ALTER TABLE kyc_profiles
  ADD COLUMN IF NOT EXISTS device_fingerprint TEXT;

CREATE INDEX IF NOT EXISTS idx_kyc_profiles_device_fingerprint
    ON kyc_profiles (device_fingerprint)
 WHERE device_fingerprint IS NOT NULL;
