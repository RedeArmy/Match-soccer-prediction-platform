-- Migration 000117: create kyc_documents table.
--
-- Each row represents one uploaded identity document attached to a KYC or KYB
-- profile. Binary content lives in the FileStore (S3/R2/OneDrive/GDrive);
-- storage_key is the opaque path returned by FileStore.Put.
--
-- profile_type distinguishes user profiles (kyc_profiles) from organisation
-- profiles (kyb_profiles, added in a future migration).
--
-- file_hash is the hex-encoded SHA-256 digest of the raw uploaded bytes,
-- computed before the file reaches the FileStore. It enables integrity
-- verification on retrieval and duplicate-document detection.
--
-- verified / verified_at / verified_by are set by the admin during review
-- to indicate that the document is authentic and matches the submitted
-- profile data.

CREATE TABLE kyc_documents (
  id            BIGSERIAL    PRIMARY KEY,
  profile_id    INT          NOT NULL,
  profile_type  TEXT         NOT NULL DEFAULT 'user'
                  CHECK (profile_type IN ('user','org')),
  document_type TEXT         NOT NULL
                  CHECK (document_type IN ('gov_id','selfie','proof_of_address','proof_of_funds')),
  storage_key   TEXT         NOT NULL,
  content_type  TEXT         NOT NULL,
  file_size     INT          NOT NULL CHECK (file_size > 0),
  file_hash     TEXT         NOT NULL DEFAULT '',
  verified      BOOLEAN      NOT NULL DEFAULT FALSE,
  verified_at   TIMESTAMPTZ,
  verified_by   INT          REFERENCES users(id) ON DELETE SET NULL,
  uploaded_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Composite FK-like index: profile_id+profile_type identify the owning profile.
CREATE INDEX idx_kyc_documents_profile ON kyc_documents (profile_id, profile_type);
CREATE INDEX idx_kyc_documents_hash    ON kyc_documents (file_hash) WHERE file_hash <> '';
