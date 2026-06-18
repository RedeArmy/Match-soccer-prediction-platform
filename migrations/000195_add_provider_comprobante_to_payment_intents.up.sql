-- Add provider, comprobante, and review columns to payment_intents so that
-- Recurrente checkouts are also tracked (provider='recurrente') and both
-- providers support an admin-managed proof-of-payment (comprobante) flow.

ALTER TABLE payment_intents
    ADD COLUMN provider                TEXT         NOT NULL DEFAULT 'paypal',
    ADD COLUMN comprobante_key         TEXT,
    ADD COLUMN comprobante_content_type TEXT,
    ADD COLUMN comprobante_file_size   INT,
    ADD COLUMN reviewed_by             INT          REFERENCES users(id),
    ADD COLUMN review_notes            TEXT         NOT NULL DEFAULT '',
    ADD COLUMN rejected_at             TIMESTAMPTZ;

-- Extend the status set to allow admin rejection of comprobantes.
ALTER TABLE payment_intents
    DROP CONSTRAINT payment_intents_status_check;
ALTER TABLE payment_intents
    ADD  CONSTRAINT payment_intents_status_check
    CHECK (status IN ('pending', 'captured', 'expired', 'rejected'));

-- Supports the admin list endpoint (filter by provider + status).
CREATE INDEX payment_intents_provider_status_idx
    ON payment_intents (provider, status);
