-- Records for bank transfer payment proofs uploaded by users in Guatemala.
--
-- Users upload a screenshot or PDF of their bank transfer receipt. An admin
-- reviews the proof, verifies the declared amount, and approves or rejects it.
-- On approval the service layer credits the user's balance atomically.
--
-- storage_key is an opaque reference to the file in the configured FileStore
-- (e.g. "proofs/2026/05/abc123.pdf"). The application never stores raw file
-- bytes in the database.
-- content_type must be one of: image/jpeg, image/png, application/pdf.

CREATE TABLE bank_transfer_proofs (
  id            BIGSERIAL    PRIMARY KEY,
  user_id       INT          NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  amount_cents  INT          NOT NULL CHECK (amount_cents > 0),
  currency      TEXT         NOT NULL DEFAULT 'GTQ',
  storage_key   TEXT         NOT NULL,
  content_type  TEXT         NOT NULL,
  file_size     INT          NOT NULL CHECK (file_size > 0),
  status        TEXT         NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'approved', 'rejected')),
  reviewed_by   INT          REFERENCES users(id) ON DELETE SET NULL,
  notes         TEXT         NOT NULL DEFAULT '',
  approved_at   TIMESTAMPTZ,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bank_transfer_proofs_user_id ON bank_transfer_proofs (user_id);
CREATE INDEX idx_bank_transfer_proofs_status  ON bank_transfer_proofs (status) WHERE status = 'pending';
