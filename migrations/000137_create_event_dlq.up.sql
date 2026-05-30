-- event_dlq persists critical domain events that exhausted all Redis Streams
-- retry attempts AND could not be written to the Redis DLQ (e.g. Redis was
-- entirely unavailable during match scoring). This table is the last-resort
-- durability layer for EventMatchFinished and EventMatchStarted so that no
-- scoring event is silently lost when the event bus is degraded.
--
-- Rows remain until an operator marks them resolved after manual replay.
-- resolved_at IS NULL = actionable; resolved_at IS NOT NULL = closed.
CREATE TABLE event_dlq (
    id           BIGSERIAL    PRIMARY KEY,
    event_type   TEXT         NOT NULL,
    payload      JSONB        NOT NULL,
    handler_err  TEXT         NOT NULL,
    attempts     SMALLINT     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    resolved_at  TIMESTAMPTZ
);

-- Narrow index for the admin query: "show me all unresolved scoring failures".
CREATE INDEX idx_event_dlq_unresolved
    ON event_dlq (event_type, created_at)
    WHERE resolved_at IS NULL;
