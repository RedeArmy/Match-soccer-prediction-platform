-- Add an operator-editable email subject template to notification_templates.
--
-- When email_subject_tmpl is non-empty the dispatcher uses it as the email
-- subject line instead of falling back to the notification title.  This lets
-- operators write distinct subjects (e.g. "Your payment of Q50.00 is confirmed")
-- without deploying a code change.
--
-- Template syntax is identical to title_tmpl / body_tmpl: Go text/template with
-- the notifTemplateFuncs helper set (formatCents, int).
--
-- DEFAULT '' means all existing rows are unchanged: the dispatcher already falls
-- back to content.title when email_subject_tmpl is empty, so no back-fill is
-- needed.
ALTER TABLE notification_templates
    ADD COLUMN email_subject_tmpl TEXT NOT NULL DEFAULT '';
