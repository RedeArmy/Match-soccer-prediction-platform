-- Migration 000122: create kyb_profiles table.
--
-- Stores KYB (Know Your Business) identity-verification data for quiniela
-- organisers — users who create and manage paid quinielas and receive payouts.
-- KYB approval is required before any organiser withdrawal is processed.
--
-- user_id is the platform user who represents the organisation. One user may
-- have at most one active KYB profile at a time (unique constraint).
--
-- tax_id + jurisdiction uniquely identify a legal entity for duplicate
-- detection. The composite index also supports the deduplication check
-- performed by KYBService.Submit before persisting a new profile.
--
-- Status values mirror KYCStatus to share the same admin review workflow.

CREATE TABLE kyb_profiles (
  id                   SERIAL       PRIMARY KEY,
  user_id              INT          NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  status               TEXT         NOT NULL DEFAULT 'unverified'
                         CHECK (status IN ('unverified','pending','under_review',
                                           'approved','rejected','expired','escalated')),
  tier                 INT          NOT NULL DEFAULT 0 CHECK (tier BETWEEN 0 AND 3),
  legal_name           TEXT         NOT NULL DEFAULT '',
  tax_id               TEXT         NOT NULL DEFAULT '',
  registration_number  TEXT         NOT NULL DEFAULT '',
  jurisdiction         TEXT         NOT NULL DEFAULT '',
  incorporation_date   DATE,
  ubo_name             TEXT         NOT NULL DEFAULT '',
  ubo_document_number  TEXT         NOT NULL DEFAULT '',
  submitted_at         TIMESTAMPTZ,
  reviewed_at          TIMESTAMPTZ,
  reviewed_by          INT          REFERENCES users(id) ON DELETE SET NULL,
  rejection_reason     TEXT         NOT NULL DEFAULT '',
  created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kyb_profiles_user_id    ON kyb_profiles (user_id);
CREATE INDEX idx_kyb_profiles_status     ON kyb_profiles (status);
CREATE UNIQUE INDEX idx_kyb_profiles_taxid_jurisdiction
  ON kyb_profiles (tax_id, jurisdiction)
  WHERE status NOT IN ('rejected', 'unverified');
