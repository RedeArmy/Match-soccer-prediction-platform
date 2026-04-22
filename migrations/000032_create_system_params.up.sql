CREATE TABLE system_params (
    key        TEXT PRIMARY KEY,
    value      TEXT        NOT NULL,
    type       TEXT        NOT NULL DEFAULT 'string'
                   CHECK (type IN ('string', 'int', 'bool', 'duration')),
    category   TEXT        NOT NULL DEFAULT 'general',
    is_runtime BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_system_params_category ON system_params (category);
