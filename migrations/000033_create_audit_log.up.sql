CREATE TABLE audit_log (
    id            BIGSERIAL   PRIMARY KEY,
    actor_id      INT         REFERENCES users (id) ON DELETE SET NULL,
    actor_role    TEXT,
    action        TEXT        NOT NULL,
    resource_type TEXT,
    resource_id   INT,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Actor lookup (admin dashboard: "show everything this user did").
CREATE INDEX idx_audit_log_actor_id    ON audit_log (actor_id);
-- Resource lookup (audit trail for a specific entity).
CREATE INDEX idx_audit_log_resource    ON audit_log (resource_type, resource_id);
-- Chronological queries and range scans.
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at);
