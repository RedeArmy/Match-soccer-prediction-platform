-- Per-user notification inbox.
--
-- Every notification dispatched to a user is persisted here so the frontend
-- can render a notification centre showing unread count and history.
-- The idempotency_key prevents the outbox worker from inserting duplicate
-- rows when it retries a previously failed dispatch attempt.
CREATE TABLE notifications (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         INT          NOT NULL REFERENCES users(id),
    event_type      TEXT         NOT NULL,
    title           TEXT         NOT NULL,
    body            TEXT         NOT NULL,
    action_url      TEXT,
    metadata        JSONB        NOT NULL DEFAULT '{}',
    idempotency_key TEXT         UNIQUE,
    read_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Supports efficient unread-count queries and the notification centre list,
-- which is always ordered newest-first and filtered to unread-only by default.
CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id, created_at DESC)
    WHERE read_at IS NULL;
