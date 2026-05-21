-- Seed the default notification locale.
--
-- All user-facing notification text (email subjects, bodies, push titles) is
-- rendered in this language.  Supported values: "en" (English), "es" (Spanish).
-- The default "es" matches the primary audience of the Guatemalan deployment.
--
-- Per-user locale is not yet supported; this param controls the system-wide
-- default.  Override at runtime via the admin system_params API without restart.
--
-- Idempotent: ON CONFLICT DO NOTHING is safe on re-run.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.default_locale',
    'es', 'es',
    'string', 'notify',
    TRUE,
    'BCP-47 language tag for user-facing notification text. Supported: "en" (English), "es" (Spanish). Default: "es". Changeable at runtime.'
)
ON CONFLICT (key) DO NOTHING;
