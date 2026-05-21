-- Prevent duplicate pending outbox events for the same entity + event type.
--
-- When a service method retries after a transient failure it may call
-- WriteInTx (or Write) a second time before the outbox worker has claimed the
-- first row.  Without this index, both rows would be dispatched and the user
-- would receive a double-notification.
--
-- The partial predicate (status = 'pending') limits the constraint to the
-- active window: once the worker transitions a row to 'processing', 'done',
-- or 'failed' the index entry is removed, allowing the same event to be
-- re-emitted in a future transaction without conflict.
--
-- Writer.WriteInTx and Writer.Write use ON CONFLICT DO NOTHING so duplicate
-- inserts are silently skipped rather than returning an error to the caller.
CREATE UNIQUE INDEX idx_outbox_pending_event_dedup
    ON domain_outbox (aggregate_type, aggregate_id, event_type)
    WHERE status = 'pending';
