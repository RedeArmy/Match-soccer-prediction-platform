DELETE FROM system_params
WHERE key IN ('kyc.ip_velocity_window_minutes', 'kyc.ip_velocity_max_submissions');
