-- Immutable audit ledger for every balance mutation on a user account.
--
-- Each row records one atomic delta and the resulting balance snapshot so
-- that the full account history can be reconstructed at any point in time.
-- Rows are only ever inserted, never updated or deleted.
--
-- kind values:
--   webhook_recurrente   - credit from a Recurrente payment webhook
--   webhook_paypal       - credit from a PayPal payment webhook
--   bank_transfer        - credit after admin approves a bank transfer proof
--   entry_fee            - debit when a user joins a paid quiniela
--   prize                - credit when a user wins a prize
--   withdrawal_reserve   - funds locked when a withdrawal request is submitted
--   withdrawal_release   - locked funds returned on withdrawal rejection
--   withdrawal_deduct    - permanent debit when a withdrawal is approved/processed
--
-- ref_id / ref_type link back to the originating record
--   (payment_records, bank_transfer_proofs, withdrawal_requests).
-- created_by is null for system/webhook-triggered mutations.

CREATE TABLE balance_ledger (
  id            BIGSERIAL    PRIMARY KEY,
  user_id       INT          NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  delta_cents   INT          NOT NULL,
  kind          TEXT         NOT NULL CHECK (kind IN (
                  'webhook_recurrente', 'webhook_paypal',
                  'bank_transfer',
                  'entry_fee', 'prize',
                  'withdrawal_reserve', 'withdrawal_release', 'withdrawal_deduct'
                )),
  balance_after INT          NOT NULL,
  ref_id        BIGINT,
  ref_type      TEXT,
  created_by    INT          REFERENCES users(id) ON DELETE SET NULL,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_balance_ledger_user_id ON balance_ledger (user_id, created_at DESC);
CREATE INDEX idx_balance_ledger_ref     ON balance_ledger (ref_type, ref_id) WHERE ref_id IS NOT NULL;
