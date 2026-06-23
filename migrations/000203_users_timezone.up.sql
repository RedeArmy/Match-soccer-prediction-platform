-- Migration 000203: add timezone column to users table.
--
-- Stores the user's preferred IANA timezone (e.g. 'America/Guatemala').
-- Used to display match kickoff times in the user's local time in emails.
-- Defaults to 'America/Guatemala' for all existing and new rows.

ALTER TABLE users ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'America/Guatemala';
