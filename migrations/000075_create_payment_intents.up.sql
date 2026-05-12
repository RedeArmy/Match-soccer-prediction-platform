-- payment_intents stores server-generated, single-use tokens that the
-- frontend embeds as custom_id when creating a PayPal order. Using an opaque
-- server-generated token instead of the raw user ID prevents a user from
-- specifying another user's ID in the PayPal order metadata and having the
-- webhook credit the wrong account.
--
-- Lifecycle: pending → captured (webhook) | expired (TTL elapsed).
-- CaptureID is the PayPal capture transaction ID; the partial unique index
-- prevents a single PayPal capture from crediting more than one intent.

CREATE TABLE payment_intents (
    id           BIGSERIAL    PRIMARY KEY,
    token        TEXT         NOT NULL,
    user_id      INT          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_cents INT          NOT NULL CHECK (amount_cents > 0),
    currency     TEXT         NOT NULL DEFAULT 'GTQ',
    status       TEXT         NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'captured', 'expired')),
    capture_id   TEXT,
    expires_at   TIMESTAMPTZ  NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Opaque token must be globally unique; used as PayPal custom_id lookup key.
CREATE UNIQUE INDEX payment_intents_token_idx
    ON payment_intents (token);

-- A PayPal capture transaction must map to at most one intent.
-- Partial: NULL capture_id (pre-capture) rows are excluded.
CREATE UNIQUE INDEX payment_intents_capture_id_idx
    ON payment_intents (capture_id)
    WHERE capture_id IS NOT NULL;

-- Supports listing a user's intents and the expiration sweep worker.
CREATE INDEX payment_intents_user_status_idx
    ON payment_intents (user_id, status);

-- Supports an expiration sweep: WHERE status = 'pending' AND expires_at < NOW().
CREATE INDEX payment_intents_expires_at_pending_idx
    ON payment_intents (expires_at)
    WHERE status = 'pending';
