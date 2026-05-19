-- Web Push (VAPID) subscriptions.
--
-- When a user grants push notification permission in their browser, the
-- frontend registers the resulting PushSubscription object (endpoint + keys)
-- via POST /api/v1/push/subscribe.  The outbox worker reads active rows for
-- the target user when SSE delivery is not possible (tab closed / offline).
--
-- A user may have multiple active subscriptions (different browsers or
-- devices).  The endpoint column is globally unique — each browser/device
-- combination produces a distinct endpoint URL.
CREATE TABLE push_subscriptions (
    id           BIGSERIAL    PRIMARY KEY,
    user_id      INT          NOT NULL REFERENCES users(id),
    endpoint     TEXT         NOT NULL UNIQUE,
    p256dh_key   TEXT         NOT NULL,
    auth_key     TEXT         NOT NULL,
    user_agent   TEXT,
    active       BOOL         NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

-- Used by the dispatcher to fetch all active subscriptions for a user in one
-- query when deciding whether to send a Web Push notification.
CREATE INDEX idx_push_sub_user_active
    ON push_subscriptions (user_id)
    WHERE active = TRUE;
