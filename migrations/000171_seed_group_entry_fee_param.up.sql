-- Seed the group-wide entry fee and default currency system parameters.
-- entry_fee_cents: Q30.00 = 3000 cents. Operators may override at runtime.
-- currency: ISO 4217 code applied to all new groups.
INSERT INTO system_params (key, value, type, category, is_runtime, default_value, description)
VALUES
  ('group.entry_fee_cents', '3000', 'int', 'group', TRUE, '3000',
   'Entry fee in cents applied to every new quiniela group. Default: 3000 (Q30.00). Changeable at runtime.'),
  ('group.currency', 'GTQ', 'string', 'group', FALSE, 'GTQ',
   'ISO 4217 currency code used for group entry fees and prize pools. Requires restart to change.')
ON CONFLICT (key) DO NOTHING;
