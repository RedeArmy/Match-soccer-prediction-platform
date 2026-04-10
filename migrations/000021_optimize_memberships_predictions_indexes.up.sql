-- Migration: add targeted indexes for membership filtering and prediction scoring
--
-- 1. group_memberships (quiniela_id, status, paid)
--    Membership queries that filter by quiniela_id frequently also discriminate
--    on status ('active', 'pending') and paid (TRUE/FALSE) — for example, when
--    enforcing the max_members cap or listing unpaid members before the
--    tournament starts. The existing idx_group_memberships_quiniela covers only
--    the leading column, leaving status and paid to be evaluated as a heap
--    filter. A composite partial index restricted to non-settled statuses
--    ('active' and 'pending') covers the common hot path while excluding 'left'
--    rows that are queried infrequently and only for audit purposes.
--    The (quiniela_id, user_id) unique constraint already handles point lookups;
--    this index targets multi-row range queries.

CREATE INDEX IF NOT EXISTS idx_group_memberships_quiniela_status_paid
    ON group_memberships (quiniela_id, status, paid)
    WHERE status IN ('active', 'pending');

-- 2. predictions (user_id, points DESC)
--    The leaderboard aggregation joins predictions on user_id and then orders by
--    points. A partial index restricted to scored rows (points IS NOT NULL)
--    reduces index size to only the predictions that contribute to rankings and
--    allows the planner to avoid a sort when deriving per-user totals.
--    Unscored predictions (points IS NULL) are omitted from the index; they
--    are never referenced in ranking queries.

CREATE INDEX IF NOT EXISTS idx_predictions_user_points
    ON predictions (user_id, points DESC)
    WHERE points IS NOT NULL;
