-- Migration 000119: add kyc_tier column to users.
--
-- Denormalises the effective KYC tier onto the users row so that the
-- KYCGate middleware can enforce tier limits with a single indexed
-- lookup instead of joining to kyc_profiles on every request.
--
-- The column is updated by KYCService whenever a status transition
-- changes the user's effective tier (approved, expired, rejected).
-- It intentionally mirrors kyc_profiles.tier; the canonical source
-- of truth for the review workflow remains kyc_profiles.
--
-- Default 0 = unverified — safe for all existing rows.

ALTER TABLE users
  ADD COLUMN kyc_tier INT NOT NULL DEFAULT 0 CHECK (kyc_tier BETWEEN 0 AND 3);

CREATE INDEX idx_users_kyc_tier ON users (kyc_tier) WHERE kyc_tier > 0;
