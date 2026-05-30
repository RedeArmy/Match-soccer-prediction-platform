DROP INDEX IF EXISTS uq_kyc_documents_profile_hash;

-- Restore the original advisory index that the up migration removed.
CREATE INDEX idx_kyc_documents_hash ON kyc_documents (file_hash) WHERE file_hash <> '';
