-- Restore the original seed from migration 000177.
TRUNCATE bank_account_types RESTART IDENTITY;

INSERT INTO bank_account_types (name, active) VALUES
    ('Cuenta Monetaria',              TRUE),
    ('Cuenta de Ahorro',              TRUE),
    ('Cuenta Empresarial Monetaria',  TRUE),
    ('Cuenta Empresarial de Ahorro',  TRUE),
    ('Cuenta de Depósito a Plazo',    TRUE)
ON CONFLICT (name) DO NOTHING;
