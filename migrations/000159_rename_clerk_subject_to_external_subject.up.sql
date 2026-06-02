-- Migration 000159: decouple the users table from the Clerk provider brand.
--
-- Context
-- -------
-- Migration 000006 added `clerk_subject` to tie each internal user to a Clerk
-- identity.  The column name bakes in the provider choice: switching to any
-- other OIDC/JWKS provider (Auth0, Cognito, self-hosted Keycloak) would require
-- a further schema migration even though the data stored is a generic opaque
-- subject string from whatever identity provider is configured.
--
-- The auth.IdentityProvider interface already abstracts JWT validation; this
-- migration completes the abstraction by making the schema provider-agnostic.
--
-- Change
-- ------
-- Rename the column, its unique constraint, and the partial lookup index so
-- that the schema speaks about "external_subject" (the JWT sub claim from the
-- external identity provider) rather than naming Clerk specifically.
--
-- This is a zero-downtime rename: PostgreSQL renames the column atomically and
-- all existing B-tree leaf entries remain valid — no data is moved or rewritten.
-- The application deployment must update all SQL references before or in the
-- same release as this migration; the only window of inconsistency is between
-- migration execution and application restart, which is covered by the rolling
-- deploy strategy already in use.
--
-- is_runtime: N/A — DDL only, no system_params row.

ALTER TABLE users RENAME COLUMN clerk_subject TO external_subject;

-- Rename the unique constraint created by migration 000006.
-- The name follows PostgreSQL convention: {table}_{column}_key.
ALTER TABLE users RENAME CONSTRAINT users_clerk_subject_key TO users_external_subject_key;

-- Rename the partial lookup index created by migration 000006.
ALTER INDEX idx_users_clerk_subject RENAME TO idx_users_external_subject;
