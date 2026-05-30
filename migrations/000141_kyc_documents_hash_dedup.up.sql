-- ATD-003: enforce SHA-256 hash deduplication for KYC documents at the DB layer.
--
-- The existing idx_kyc_documents_hash advisory index (non-unique) was a query
-- hint only; it did not prevent a profile from uploading the same file bytes
-- multiple times. The new unique index closes the gap:
--
--   UNIQUE (profile_id, profile_type, file_hash) WHERE file_hash <> ''
--
-- Scope: scoped to the owning profile (profile_id + profile_type) so two
-- different users can independently upload documents with identical content
-- (e.g., the same government-ID template) without conflict. A single profile
-- cannot upload the same raw bytes twice, regardless of the declared
-- document_type — if the hashes match, it is a duplicate submission.
--
-- Partial predicate: rows with file_hash = '' (legacy rows inserted before
-- hash computation was implemented) are excluded and remain unaffected.
--
-- The old non-unique advisory index is dropped because the unique index below
-- creates an equivalent B-tree that also satisfies all queries that used the
-- advisory index.

DROP INDEX IF EXISTS idx_kyc_documents_hash;

CREATE UNIQUE INDEX uq_kyc_documents_profile_hash
    ON kyc_documents (profile_id, profile_type, file_hash)
    WHERE file_hash <> '';
