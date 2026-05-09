-- Allow payment_records.user_id to be NULL so that a GDPR erasure request can
-- anonymise the financial record without deleting it. GAAP and SAT require
-- payment rows to be retained; the user link must be erasable independently.
ALTER TABLE payment_records
    ALTER COLUMN user_id DROP NOT NULL;

-- Replace ON DELETE RESTRICT with ON DELETE SET NULL so that the automated
-- hard-delete purge (after the retention window) automatically clears the
-- user link on any record that EraseUserPII missed.
ALTER TABLE payment_records
    DROP CONSTRAINT payment_records_user_id_fkey;

ALTER TABLE payment_records
    ADD CONSTRAINT payment_records_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
