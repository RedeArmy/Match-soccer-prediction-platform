-- Notification dead-letter queue.
--
-- When the outbox worker fails to deliver a notification to a specific channel
-- after all retry attempts are exhausted, it writes a DLQ row for later
-- inspection and manual or automated replay.
--
-- channel identifies the delivery channel that failed: 'email', 'push', or 'sse'.
-- resolved_at is set when the entry is successfully replayed or manually dismissed.
CREATE TABLE notification_dlq (
    id            BIGSERIAL    PRIMARY KEY,
    outbox_id     BIGINT       REFERENCES domain_outbox(id) ON DELETE SET NULL,
    channel       TEXT         NOT NULL CHECK (channel IN ('email', 'push', 'sse')),
    user_id       INT          REFERENCES users(id) ON DELETE SET NULL,
    event_type    TEXT         NOT NULL,
    payload       JSONB        NOT NULL,
    error_detail  TEXT         NOT NULL,
    attempts      SMALLINT     NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_retry_at TIMESTAMPTZ,
    resolved_at   TIMESTAMPTZ
);

-- Used by the DLQ retry worker to claim unresolved entries ordered by age.
CREATE INDEX idx_dlq_unresolved ON notification_dlq (created_at)
    WHERE resolved_at IS NULL;
