-- Migration 000176: Guatemala financial system banks lookup table.
--
-- gt_banks stores the banks available as withdrawal destinations. The active
-- column lets operators disable a bank (e.g. after a merger) without a code
-- change. The frontend fetches GET /api/v1/banks and renders the list as a
-- dropdown on the withdraw page.

CREATE TABLE gt_banks (
    id         SERIAL PRIMARY KEY,
    name       TEXT    NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT gt_banks_name_unique UNIQUE (name)
);

CREATE INDEX idx_gt_banks_active ON gt_banks (active) WHERE active = TRUE;

-- Seed: all banks supervised by the Superintendencia de Bancos de Guatemala (SIB).
INSERT INTO gt_banks (name, active) VALUES
    ('Banco Industrial, S.A.',                    TRUE),
    ('Banco G&T Continental, S.A.',               TRUE),
    ('Banco Agromercantil de Guatemala, S.A.',    TRUE),
    ('Banco de Desarrollo Rural, S.A.',           TRUE),
    ('Banco Internacional, S.A.',                 TRUE),
    ('Banco de América Central, S.A.',            TRUE),
    ('Banco Promerica de Guatemala, S.A.',        TRUE),
    ('Banco Ficohsa Guatemala, S.A.',             TRUE),
    ('Banco de los Trabajadores',                 TRUE),
    ('Crédito Hipotecario Nacional',              TRUE),
    ('Banco WestTrust, S.A.',                     TRUE),
    ('Banco Inmobiliario, S.A.',                  TRUE),
    ('Banco Azteca de Guatemala, S.A.',           TRUE),
    ('Vivibanco, S.A.',                           TRUE),
    ('Citibank, N.A. Sucursal Guatemala',         TRUE)
ON CONFLICT (name) DO NOTHING;
