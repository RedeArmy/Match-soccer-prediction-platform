-- Migration 000135: add locale preference column to users.
--
-- locale stores the user's preferred BCP-47 language tag for all user-facing
-- strings: API error messages, notification titles and bodies, email copy, and
-- push payloads. Supported values are 'en' (English) and 'es' (Spanish).
--
-- DEFAULT 'es' matches the primary Guatemalan audience. Every existing user
-- row is set to 'es' automatically; a user can update it via
-- PATCH /api/v1/users/me once the front-end exposes the preference UI.
--
-- The CHECK constraint rejects any value outside the two supported tags so
-- the application layer never needs to handle unknown locales at runtime.

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS locale VARCHAR(10) NOT NULL DEFAULT 'es'
  CHECK (locale IN ('en', 'es'));
