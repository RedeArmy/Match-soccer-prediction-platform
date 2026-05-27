-- Seed system parameters for KYC IP submission velocity checks.
-- kyc.ip_velocity_window_minutes: rolling window for counting submissions per IP.
-- kyc.ip_velocity_max_submissions: maximum allowed submissions within the window.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('kyc.ip_velocity_window_minutes', '60', '60', 'int', 'kyc', true,
     'Rolling window in minutes for counting KYC submissions from a single IP address.'),
    ('kyc.ip_velocity_max_submissions', '3', '3', 'int', 'kyc', true,
     'Maximum KYC submissions from one IP within the velocity window before requests are rate-limited.')
ON CONFLICT (key) DO NOTHING;
