-- Migration: optimize quinielas index coverage
--
-- Two gaps identified by query analysis on the quinielas table:
--
-- 1. invite_code lookup (GetByInviteCode)
--    The UNIQUE constraint on invite_code creates an implicit index on the full
--    table, but the application query always adds "AND deleted_at IS NULL".
--    PostgreSQL cannot use the constraint index to satisfy that predicate
--    efficiently and falls back to a sequential scan. A dedicated partial index
--    restricted to active rows closes this gap. This index is hit on every
--    "join group" request and is therefore high priority.
--
-- 2. owner listing (ListByOwner)
--    The existing idx_quinielas_owner_id (owner_id) covers the filter but not
--    the ORDER BY created_at DESC, forcing a sort step on every call. Replacing
--    it with a covering partial index on (owner_id, created_at DESC) WHERE
--    deleted_at IS NULL lets the planner satisfy both the filter and the sort
--    from the index alone, eliminating the sort node entirely.

-- Close the invite_code lookup gap.
CREATE INDEX IF NOT EXISTS idx_quinielas_invite_code
    ON quinielas (invite_code)
    WHERE deleted_at IS NULL;

-- Replace the single-column owner index with a composite partial index that
-- also covers the ORDER BY created_at DESC used by ListByOwner.
DROP INDEX IF EXISTS idx_quinielas_owner_id;

CREATE INDEX IF NOT EXISTS idx_quinielas_owner_created
    ON quinielas (owner_id, created_at DESC)
    WHERE deleted_at IS NULL;
