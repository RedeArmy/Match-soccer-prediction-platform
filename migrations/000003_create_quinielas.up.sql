-- Migration: create quinielas table
CREATE TABLE IF NOT EXISTS quinielas (
    id         SERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    owner_id   INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_quinielas_owner_id ON quinielas (owner_id);
