-- Track when a push subscription was deactivated so the pruning job can
-- enforce a retention window based on inactivation time rather than creation
-- time.  Existing rows that are already inactive keep inactivated_at = NULL;
-- the pruner falls back to created_at for those rows via COALESCE.
ALTER TABLE push_subscriptions
    ADD COLUMN inactivated_at TIMESTAMPTZ;

-- Partial index for the pruning query: only inactive rows are candidates.
CREATE INDEX idx_push_sub_inactive_prune
    ON push_subscriptions (inactivated_at)
    WHERE active = FALSE;
