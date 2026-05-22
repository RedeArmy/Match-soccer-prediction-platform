-- Migration 000104: audit history for notification_templates.
--
-- Problem: notification_templates.Upsert is destructive — the previous
-- content is overwritten with no trace.  A typo in a template requires
-- manual DB intervention to recover the last known-good state.
--
-- Solution: a BEFORE UPDATE trigger copies every old row into
-- notification_template_history before each Upsert overwrites it.
-- The history table stores all previous versions; the most recent is
-- always the highest id for that (event_type, locale) pair.
--
-- Rollback: the admin API reads a history entry and upserts it back
-- to notification_templates (which archives the current live row again),
-- giving a complete, non-destructive audit trail.

CREATE TABLE notification_template_history (
    id                BIGSERIAL   PRIMARY KEY,
    event_type        TEXT        NOT NULL,
    locale            TEXT        NOT NULL,
    title_tmpl        TEXT        NOT NULL,
    body_tmpl         TEXT        NOT NULL,
    action_url_tmpl   TEXT        NOT NULL DEFAULT '',
    email_subject_tmpl TEXT       NOT NULL DEFAULT '',
    email_html_tmpl   TEXT        NOT NULL DEFAULT '',
    changed_by        INTEGER     REFERENCES users(id) ON DELETE SET NULL,
    changed_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE notification_template_history IS
    'Immutable audit log of every previous notification_templates state. '
    'Rows are inserted automatically by the notification_templates_archive trigger '
    'before each UPDATE.  The row with the highest id is the most recent prior version.';

CREATE INDEX ON notification_template_history (event_type, locale, id DESC);

-- Trigger function: archive the old row before each UPDATE.
CREATE OR REPLACE FUNCTION notification_templates_archive()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO notification_template_history
        (event_type, locale, title_tmpl, body_tmpl, action_url_tmpl,
         email_subject_tmpl, email_html_tmpl, changed_by, changed_at)
    VALUES
        (OLD.event_type, OLD.locale, OLD.title_tmpl, OLD.body_tmpl, OLD.action_url_tmpl,
         COALESCE(OLD.email_subject_tmpl, ''), COALESCE(OLD.email_html_tmpl, ''),
         OLD.updated_by, OLD.updated_at);
    RETURN NEW;
END;
$$;

CREATE TRIGGER notification_templates_archive
    BEFORE UPDATE ON notification_templates
    FOR EACH ROW
    EXECUTE FUNCTION notification_templates_archive();
