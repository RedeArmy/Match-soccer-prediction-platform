DELETE FROM system_params
WHERE key IN (
    'payment.bank_transfer_min_amount_cents',
    'payment.bank_transfer_max_amount_cents'
);
