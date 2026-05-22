-- Migration 000106: audit history for system_params mutations.
--
-- Problem: system_params.Set is destructive — the previous value is
-- overwritten with no trace. The audit_log records {key, new_value} but
-- stores neither the old_value nor an indexed key, making "who changed X
-- from Y to Z and when?" impossible to answer without JSON parsing.
--
-- Solution: a dedicated system_params_history table records every Set and
-- ResetToDefault call at the service layer. Each row stores both the
-- old and new value together with the actor and wall-clock timestamp.
--
-- Rollback: the admin API reads a history entry and calls Set with old_value
-- to restore the previous state (which itself produces a new history row).

CREATE TABLE system_params_history (
    id          BIGSERIAL   PRIMARY KEY,
    key         TEXT        NOT NULL REFERENCES system_params(key) ON DELETE CASCADE,
    old_value   TEXT        NOT NULL,
    new_value   TEXT        NOT NULL,
    actor_id    INTEGER     REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT        NOT NULL,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT system_params_history_action_check
        CHECK (action IN ('set', 'reset'))
);

COMMENT ON TABLE system_params_history IS
    'Append-only audit trail of every system_params value change. '
    'Rows are written by the service layer on each Set or ResetToDefault call. '
    'The row with the highest id for a given key is the most recent change.';

CREATE INDEX ON system_params_history (key, id DESC);

-- Seed the retention-policy param so operators can adjust it via the admin API.
-- Default: 90 days, covering the FIFA 2026 tournament window plus two months.
-- is_runtime=FALSE because the worker reads it once at startup via GetInt.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'system.param_history_retention_days',
    '90', '90',
    'int', 'system',
    FALSE,
    'Days to retain system_params_history rows before the daily purge job deletes them. Restart the worker to apply a new value.'
)
ON CONFLICT (key) DO NOTHING;
