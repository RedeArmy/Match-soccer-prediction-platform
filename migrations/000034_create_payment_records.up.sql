CREATE TABLE payment_records (
    id           BIGSERIAL   PRIMARY KEY,
    quiniela_id  INT         NOT NULL REFERENCES quinielas (id) ON DELETE RESTRICT,
    user_id      INT         NOT NULL REFERENCES users (id)     ON DELETE RESTRICT,
    amount       INT         NOT NULL CHECK (amount >= 0),
    currency     TEXT        NOT NULL DEFAULT 'MXN',
    status       TEXT        NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'confirmed', 'refunded', 'rejected')),
    reference    TEXT,                            -- external provider transaction ID
    reviewed_by  INT         REFERENCES users (id) ON DELETE SET NULL,
    notes        TEXT        NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_records_quiniela_id ON payment_records (quiniela_id);
CREATE INDEX idx_payment_records_user_id     ON payment_records (user_id);
-- Partial index: only pending payments are candidates for reconciliation jobs.
CREATE INDEX idx_payment_records_status      ON payment_records (status)
    WHERE status = 'pending';
