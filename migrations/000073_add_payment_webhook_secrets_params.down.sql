DELETE FROM system_params
WHERE key IN (
  'payment.max_upload_bytes',
  'payment.withdrawal_min_cents',
  'payment.withdrawal_max_cents'
);
