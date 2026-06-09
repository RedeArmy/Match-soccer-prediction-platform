-- Migration 000177: bank account types lookup table.
--
-- bank_account_types stores the account types accepted for withdrawal deposits.
-- The active column lets operators disable a type without a code change.
-- The frontend fetches GET /api/v1/bank-account-types and renders the list
-- as a dropdown on the withdraw page.

CREATE TABLE bank_account_types (
    id         SERIAL PRIMARY KEY,
    name       TEXT    NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bank_account_types_name_unique UNIQUE (name)
);

CREATE INDEX idx_bank_account_types_active ON bank_account_types (active) WHERE active = TRUE;

-- Seed: account types used by the Guatemalan financial system.
INSERT INTO bank_account_types (name, active) VALUES
    ('Cuenta Monetaria',              TRUE),
    ('Cuenta de Ahorro',              TRUE),
    ('Cuenta Empresarial Monetaria',  TRUE),
    ('Cuenta Empresarial de Ahorro',  TRUE),
    ('Cuenta de Depósito a Plazo',    TRUE)
ON CONFLICT (name) DO NOTHING;
