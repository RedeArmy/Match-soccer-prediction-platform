-- Migration 000116: create kyc_profiles table.
--
-- Stores the identity-verification profile for each user. One row per user;
-- the row is upserted in place on resubmission. The full state-transition
-- history is captured in kyc_events (migration 000118).
--
-- Tier values mirror domain.KYCTier: 0=unverified,1=low,2=medium,3=high.
-- status is the lifecycle position within that tier's review workflow.
--
-- balance_frozen / frozen_amount_cents / frozen_reason track the large-win
-- hold: when a prize credit exceeds kyc.win_freeze_threshold_cents and the
-- user's tier is below the required level, the balance is frozen here until
-- an admin releases it post-approval.
--
-- pep_flag and sanctions_flag are initially set by manual admin action;
-- they are designed to also be written by a future third-party screening
-- provider (see KYCProvider interface in internal/kyc/provider.go).
--
-- next_review_at enables periodic re-verification: the notification scheduler
-- sends a reminder 30 days before this date, and the admin can mark the
-- profile as expired after it passes.

CREATE TABLE kyc_profiles (
  id                   SERIAL       PRIMARY KEY,
  user_id              INT          NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  status               TEXT         NOT NULL DEFAULT 'unverified'
                         CHECK (status IN ('unverified','pending','under_review',
                                           'approved','rejected','expired','escalated')),
  tier                 INT          NOT NULL DEFAULT 0 CHECK (tier BETWEEN 0 AND 3),

  -- Personal identity data
  full_name            TEXT         NOT NULL DEFAULT '',
  date_of_birth        DATE,
  nationality          TEXT         NOT NULL DEFAULT '',
  document_type        TEXT         CHECK (document_type IN ('gov_id','selfie','proof_of_address','proof_of_funds')),
  document_number      TEXT         NOT NULL DEFAULT '',
  address_line         TEXT         NOT NULL DEFAULT '',
  city                 TEXT         NOT NULL DEFAULT '',
  country              TEXT         NOT NULL DEFAULT '',
  postal_code          TEXT         NOT NULL DEFAULT '',

  -- Workflow
  submitted_at         TIMESTAMPTZ,
  reviewed_at          TIMESTAMPTZ,
  reviewed_by          INT          REFERENCES users(id) ON DELETE SET NULL,
  rejection_reason     TEXT         NOT NULL DEFAULT '',

  -- Risk
  risk_score           INT          NOT NULL DEFAULT 0 CHECK (risk_score BETWEEN 0 AND 100),
  pep_flag             BOOLEAN      NOT NULL DEFAULT FALSE,
  sanctions_flag       BOOLEAN      NOT NULL DEFAULT FALSE,

  -- Balance freeze
  balance_frozen       BOOLEAN      NOT NULL DEFAULT FALSE,
  frozen_amount_cents  INT          NOT NULL DEFAULT 0 CHECK (frozen_amount_cents >= 0),
  frozen_reason        TEXT         NOT NULL DEFAULT '',

  -- Periodic re-verification
  next_review_at       TIMESTAMPTZ,

  created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kyc_profiles_status     ON kyc_profiles (status) WHERE status IN ('pending','under_review','escalated');
CREATE INDEX idx_kyc_profiles_frozen     ON kyc_profiles (balance_frozen) WHERE balance_frozen = TRUE;
CREATE INDEX idx_kyc_profiles_review_at  ON kyc_profiles (next_review_at) WHERE next_review_at IS NOT NULL;
