ALTER TABLE users RENAME COLUMN external_subject TO clerk_subject;
ALTER TABLE users RENAME CONSTRAINT users_external_subject_key TO users_clerk_subject_key;
ALTER INDEX idx_users_external_subject RENAME TO idx_users_clerk_subject;
