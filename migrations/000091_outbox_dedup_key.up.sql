-- Add a per-row deduplication key to the notification outbox.
--
-- Scheduler jobs that may fire multiple times within a single "reminder window"
-- (e.g. PredictionDeadlineApproaching running every 5 minutes while a match is
-- within 60 minutes of kickoff) use Writer.WriteDedup with a stable key that
-- encodes the event's uniqueness scope.  The partial unique index ensures that
-- only one pending/processing row can exist per key at any time, while allowing
-- the same key to be reused once an old row has been processed (done/failed).
--
-- Rows written via the non-dedup path (Write / WriteInTx / WriteBatch) leave
-- dedup_key NULL; PostgreSQL treats NULLs as distinct in unique indexes, so
-- those rows never conflict with each other.
ALTER TABLE domain_outbox ADD COLUMN dedup_key TEXT;

CREATE UNIQUE INDEX idx_outbox_dedup
    ON domain_outbox (dedup_key)
    WHERE dedup_key IS NOT NULL;
