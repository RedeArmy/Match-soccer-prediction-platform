-- Migration 000145: partial index on users.deleted_at for KYC document purge.
--
-- ListExpiredDocuments (kyc_document_repository.go) joins kyc_documents →
-- kyc_profiles → users and filters WHERE u.deleted_at IS NOT NULL AND
-- u.deleted_at < $1. Without this index PostgreSQL must scan the full users
-- table on each weekly purge run. The existing idx_users_active index
-- (WHERE deleted_at IS NULL) is irrelevant for this predicate.
--
-- CONCURRENTLY: avoids an exclusive lock on the users table; the migration is
-- safe to apply under live traffic (see 000127 for precedent).

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_deleted_at_partial
    ON users (deleted_at)
    WHERE deleted_at IS NOT NULL;
