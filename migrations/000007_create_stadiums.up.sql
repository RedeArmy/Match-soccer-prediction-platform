-- Migration: create stadiums table
--
-- Stadiums are the 16 official FIFA World Cup 2026 venues across the USA,
-- Canada, and Mexico. This is reference data that changes extremely rarely
-- (only if a host city is added or withdrawn), so capacity is stored here
-- for display purposes rather than as a business invariant.
CREATE TABLE IF NOT EXISTS stadiums (
    id         SERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    city       TEXT        NOT NULL,
    country    TEXT        NOT NULL,
    capacity   INTEGER     NOT NULL CHECK (capacity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
