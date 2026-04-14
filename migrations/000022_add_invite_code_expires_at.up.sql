-- Add optional expiry for quiniela invite codes.
--
-- invite_code_expires_at is NULL by default, meaning the code never expires
-- unless the owner explicitly rotates it. When set, GetByInviteCode rejects
-- the code after this timestamp, forcing new members to request a fresh code.
--
-- Index note: PostgreSQL partial index predicates require IMMUTABLE functions.
-- NOW() is STABLE (not IMMUTABLE) so it cannot appear in a predicate.
-- The index therefore covers all active (non-deleted) rows regardless of
-- expiry, and the expiry filter is applied at query time in GetByInviteCode.
-- This is the correct trade-off: expired codes are rare, so the extra heap
-- fetch for expired rows has negligible cost.

ALTER TABLE quinielas
    ADD COLUMN invite_code_expires_at TIMESTAMPTZ NULL;

-- Rebuild the invite_code index from migration 000019 as a composite index
-- that also covers invite_code_expires_at, enabling index-only scans on the
-- GetByInviteCode query which filters on both columns.
DROP INDEX IF EXISTS idx_quinielas_invite_code;

CREATE INDEX idx_quinielas_invite_code_active
    ON quinielas (invite_code, invite_code_expires_at)
    WHERE deleted_at IS NULL;
