-- Migration 000130: remove device_fingerprint from kyc_profiles.
--
-- The device_fingerprint column was introduced in migration 000123 to detect
-- account-farming fraud by correlating a client-supplied device identifier.
-- It was replaced by server-side IP velocity checks (migration 000128 +
-- CheckIPSubmissionVelocity in KYCGate) because client-supplied fingerprints
-- are trivially spoofed and provided no reliable signal.
--
-- The column has not been written to since the app-layer removal was deployed.
-- A 30-day observation window confirmed zero new writes; the column is now safe
-- to drop. The Deprecated concrete method CountAccountsByDeviceFingerprint
-- (not in the KYCProfileRepository interface) must be removed alongside this.

DROP INDEX IF EXISTS idx_kyc_profiles_device_fingerprint;

ALTER TABLE kyc_profiles
  DROP COLUMN IF EXISTS device_fingerprint;
