-- Migration 000103: seed system parameter for the notification "From:" address.
--
-- notify.from_address — the "From:" header value stamped on all outgoing
--   notification emails (user-facing and admin alerts).  When non-empty, this
--   param overrides the WCQ_EMAIL_FROMADDRESS environment variable at send time
--   without requiring a process restart (is_runtime=TRUE).
--
-- Default: '' (empty) — the process falls back to WCQ_EMAIL_FROMADDRESS so
-- existing deployments are unaffected.  Operators set a value in the admin
-- panel to override the sending domain without a deploy.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.from_address',
    '', '',
    'string', 'notify',
    TRUE,
    'Sender "From:" header for all outgoing emails. Overrides WCQ_EMAIL_FROMADDRESS when non-empty. Format: "Display Name <email@example.com>".'
)
ON CONFLICT (key) DO NOTHING;
