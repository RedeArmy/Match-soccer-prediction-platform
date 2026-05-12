-- Seed system_params rows for payment configuration.
--
-- These parameters are operator-tunable at runtime via the admin API
-- (PATCH /api/v1/admin/system-params/{key}) without a service restart.

INSERT INTO system_params (key, value, default_value, type, category, description, is_runtime)
VALUES
  ('payment.max_upload_bytes',   '5242880', '5242880', 'int',    'payment', 'Maximum allowed file size in bytes for bank transfer proof uploads (default 5 MB)', TRUE),
  ('payment.withdrawal_min_cents', '5000',  '5000',    'int',    'payment', 'Minimum withdrawal amount in minor currency units (default 5000 = 50 GTQ)', TRUE),
  ('payment.withdrawal_max_cents', '500000','500000',  'int',    'payment', 'Maximum withdrawal amount in minor currency units (default 500000 = 5000 GTQ)', TRUE)
ON CONFLICT (key) DO NOTHING;
