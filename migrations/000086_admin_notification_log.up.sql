-- Audit log for admin email notifications.
--
-- Every email dispatched to admin recipients is recorded here, including the
-- Resend message ID for traceability.  This table is append-only; rows are
-- never updated or deleted.  It is used by the admin dashboard to verify that
-- P0 alerts were delivered and to diagnose gaps in the alert pipeline.
CREATE TABLE admin_notification_log (
    id            BIGSERIAL    PRIMARY KEY,
    event_type    TEXT         NOT NULL,
    recipients    TEXT[]       NOT NULL,
    subject       TEXT         NOT NULL,
    status        TEXT         NOT NULL CHECK (status IN ('sent', 'failed')),
    resend_msg_id TEXT,
    error_detail  TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Used by the admin dashboard and the escalation cron to inspect recent
-- alert delivery history for a specific event type.
CREATE INDEX idx_admin_notif_log_event
    ON admin_notification_log (event_type, created_at DESC);
