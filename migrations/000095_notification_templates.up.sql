-- Operator-editable notification content store.
--
-- Each row overrides the compiled Go default for a (event_type, locale) pair.
-- Rows that do not exist fall back to the compiled default, so the system
-- degrades gracefully on a fresh deployment before the seed migration runs.
--
-- title_tmpl, body_tmpl, and action_url_tmpl are Go text/template strings.
-- Template data = the outbox payload decoded as map[string]any (JSON keys are
-- snake_case).  The following functions are available in all templates:
--
--   formatCents .amount_cents .currency  →  "50.00 GTQ"
--   int .match_id                        →  int64, safe for use in URL paths
--
-- Whole-number floats render correctly via {{.minutes_left}} but IDs and URL
-- path segments should always use {{int .field}} to avoid scientific notation
-- for values ≥ 1 000 000.
CREATE TABLE notification_templates (
    event_type      TEXT        NOT NULL,
    locale          TEXT        NOT NULL DEFAULT 'en',
    title_tmpl      TEXT        NOT NULL,
    body_tmpl       TEXT        NOT NULL,
    action_url_tmpl TEXT        NOT NULL DEFAULT '',
    updated_by      INTEGER     REFERENCES users(id) ON DELETE SET NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_type, locale)
);

COMMENT ON TABLE notification_templates IS
    'Operator-editable Go text/template content for user-facing notifications. '
    'Each row overrides the compiled default for (event_type, locale). '
    'Delete a row to revert to the compiled Go fallback without redeploying.';
