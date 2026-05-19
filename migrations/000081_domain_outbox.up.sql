-- Transactional outbox for the notification subsystem.
--
-- Every domain event that must trigger a notification is written to this table
-- inside the same PostgreSQL transaction as the originating domain change.
-- The outbox worker polls pending rows, dispatches them to the appropriate
-- delivery channels, and marks each row done (or failed after max_attempts).
--
-- SELECT FOR UPDATE SKIP LOCKED is used during polling so that multiple worker
-- replicas can claim disjoint batches without blocking each other.
CREATE TABLE domain_outbox (
    id             BIGSERIAL    PRIMARY KEY,
    event_type     TEXT         NOT NULL,
    aggregate_id   TEXT         NOT NULL,
    aggregate_type TEXT         NOT NULL,
    payload        JSONB        NOT NULL,
    status         TEXT         NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    attempts       SMALLINT     NOT NULL DEFAULT 0,
    max_attempts   SMALLINT     NOT NULL DEFAULT 5,
    scheduled_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    locked_until   TIMESTAMPTZ,
    processed_at   TIMESTAMPTZ,
    error_detail   TEXT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Used by the worker to claim the next batch of pending rows efficiently.
CREATE INDEX idx_outbox_poll ON domain_outbox (scheduled_at)
    WHERE status = 'pending';

-- Used by the stale-lock recovery job to find processing rows whose lock
-- expired without being committed, indicating a crashed worker instance.
CREATE INDEX idx_outbox_stale ON domain_outbox (locked_until)
    WHERE status = 'processing';
