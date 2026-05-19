-- Per-user notification channel preferences.
--
-- Each row controls whether a specific event type is delivered to a user via
-- each available channel.  Missing rows are treated as all-channels-enabled by
-- the dispatcher (safe default: opt-out model).
--
-- The primary key (user_id, event_type) enforces one preference record per
-- user per event type.  Rows are upserted via PATCH /api/v1/notifications/preferences.
CREATE TABLE notification_preferences (
    user_id        INT          NOT NULL REFERENCES users(id),
    event_type     TEXT         NOT NULL,
    channel_email  BOOL         NOT NULL DEFAULT TRUE,
    channel_push   BOOL         NOT NULL DEFAULT TRUE,
    channel_inapp  BOOL         NOT NULL DEFAULT TRUE,
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, event_type)
);
