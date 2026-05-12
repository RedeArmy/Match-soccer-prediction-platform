-- User-initiated payout requests.
--
-- On submission: amount_cents is reserved (moved to users.reserved_cents).
-- On admin approval: the reservation is committed (balance_cents decremented).
-- On admin rejection: the reservation is released (reserved_cents decremented).
--
-- method:
--   bank_gt  - Guatemalan bank account; payout_details contains account_number and bank_name.
--   paypal   - International; payout_details contains paypal_email.
--
-- payout_details is JSONB to accommodate differing fields per method without
-- separate columns. The application validates the required keys at the service
-- layer before persisting.

CREATE TABLE withdrawal_requests (
  id             BIGSERIAL    PRIMARY KEY,
  user_id        INT          NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  amount_cents   INT          NOT NULL CHECK (amount_cents > 0),
  currency       TEXT         NOT NULL,
  method         TEXT         NOT NULL CHECK (method IN ('bank_gt', 'paypal')),
  payout_details JSONB        NOT NULL DEFAULT '{}',
  status         TEXT         NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'approved', 'rejected', 'processed')),
  reviewed_by    INT          REFERENCES users(id) ON DELETE SET NULL,
  notes          TEXT         NOT NULL DEFAULT '',
  processed_at   TIMESTAMPTZ,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_withdrawal_requests_user_id ON withdrawal_requests (user_id);
CREATE INDEX idx_withdrawal_requests_status  ON withdrawal_requests (status)
  WHERE status IN ('pending', 'approved');
