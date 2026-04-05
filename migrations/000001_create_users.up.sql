-- Migration: create users table
--
-- The role column uses a check constraint rather than a PostgreSQL enum type
-- so that new roles can be added with a simple ALTER TABLE CHECK and without
-- requiring a full enum ALTER (which takes an ACCESS EXCLUSIVE lock in older
-- PostgreSQL versions).
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    name          TEXT        NOT NULL,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'player'
                  CHECK (role IN ('admin', 'player')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
