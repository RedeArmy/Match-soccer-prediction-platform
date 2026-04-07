-- Migration: add clerk_subject to users table
--
-- clerk_subject stores the opaque identifier Clerk assigns to each user
-- (format "user_<base58>"). It is used by the user-sync webhook to upsert
-- users and by the prediction handler to resolve a Clerk JWT subject to an
-- internal user ID without requiring a numeric parse.
--
-- The column is nullable to remain backward-compatible with rows seeded
-- before Clerk integration was added. In practice every new user created
-- via the Clerk webhook will have this column populated.
--
-- The partial index on non-null values keeps lookup fast without wasting
-- index space on legacy rows that still have a NULL clerk_subject.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS clerk_subject TEXT UNIQUE;

CREATE INDEX IF NOT EXISTS idx_users_clerk_subject
    ON users (clerk_subject)
    WHERE clerk_subject IS NOT NULL;
