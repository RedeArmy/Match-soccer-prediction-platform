DROP INDEX IF EXISTS idx_push_sub_inactive_prune;
ALTER TABLE push_subscriptions DROP COLUMN inactivated_at;
