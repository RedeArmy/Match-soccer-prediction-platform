-- Migration 000179: seed payment.withdrawal_min_usd_cents system parameter.
--
-- When a withdrawal is submitted with currency='USD' the service enforces a
-- separate minimum that is independent of the GTQ minimum
-- (payment.withdrawal_min_cents). Keeping them apart lets operators tune each
-- threshold without coupling them through an exchange-rate conversion.
--
-- Default: 400 USD cents ($4.00), roughly equivalent to Q30 at the ~Q7.75/$1
-- reference rate. is_runtime=TRUE: takes effect within the 30-second cache
-- window; no restart required.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'payment.withdrawal_min_usd_cents',
    '400',
    '400',
    'int',
    'payment',
    TRUE,
    'Minimum withdrawal amount in USD minor units (cents). Applied only when the '
    'withdrawal request carries currency=''USD''. Default: 400 ($4.00). '
    'Valid range: 1–100000 ($0.01–$1000.00).'
)
ON CONFLICT (key) DO NOTHING;
