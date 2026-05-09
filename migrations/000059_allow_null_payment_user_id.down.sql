-- Revert: restore NOT NULL constraint and ON DELETE RESTRICT behaviour.
-- Any records with user_id = NULL must be corrected before running this rollback.
ALTER TABLE payment_records
    DROP CONSTRAINT payment_records_user_id_fkey;

ALTER TABLE payment_records
    ADD CONSTRAINT payment_records_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE payment_records
    ALTER COLUMN user_id SET NOT NULL;
