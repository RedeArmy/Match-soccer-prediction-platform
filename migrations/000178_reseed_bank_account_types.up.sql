-- Migration 000178: replace bank_account_types seed with the correct set.
TRUNCATE bank_account_types RESTART IDENTITY;

INSERT INTO bank_account_types (name, active) VALUES
    ('Ahorros GTQ',       TRUE),
    ('Ahorros USD',       TRUE),
    ('Monetaria GTQ',     TRUE),
    ('Monetaria USD',     TRUE),
    ('Tarjeta de Crédito', TRUE)
ON CONFLICT (name) DO NOTHING;
