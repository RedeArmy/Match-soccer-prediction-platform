-- Migration 000123: add device_fingerprint to kyc_profiles.
--
-- Captures a hashed device identifier submitted by the client during KYC
-- submission. The fingerprint is computed client-side (browser fingerprint
-- or mobile device ID) and sent in the X-Device-Fingerprint header.
--
-- The compliance team uses this field to detect account farming: one physical
-- device being used to submit KYC for multiple distinct user accounts, which
-- is a pattern associated with synthetic identity fraud.
--
-- The value is stored as a SHA-256 hex digest (64 characters). NULL means
-- the client did not submit a fingerprint (older API clients, or submissions
-- made before this migration was deployed).
--
-- A partial index is created to speed up the CountAccountsByDeviceFingerprint
-- query used during KYC submission to detect multi-account fraud.

ALTER TABLE kyc_profiles
  ADD COLUMN device_fingerprint TEXT;

CREATE INDEX idx_kyc_profiles_device_fingerprint
    ON kyc_profiles (device_fingerprint)
 WHERE device_fingerprint IS NOT NULL;
