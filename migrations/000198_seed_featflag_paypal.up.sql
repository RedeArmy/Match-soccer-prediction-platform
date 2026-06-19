-- Migration 000198: seed featflag.paypal system parameter.
--
-- Controls whether the PayPal payment tab is shown on the deposit page.
-- Value '0' hides PayPal; '1' shows it. Defaults to '0' (hidden) because
-- the PayPal merchant account requires resolution before re-enabling.
--
-- ON CONFLICT DO NOTHING: safe to re-run against instances that already have
-- a manual override in place.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('featflag.paypal', '0', '0', 'bool', 'featflag', TRUE,
     'Feature flag: show PayPal payment tab on the deposit page. 0 = hidden, 1 = shown. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;
