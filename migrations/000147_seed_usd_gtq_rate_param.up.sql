-- Migration 000147: seed payment.usd_gtq_rate system parameter.
--
-- GTQ centavos per USD dollar used to convert USD PayPal withdrawal amounts
-- into the GTQ centavos reserved from the user's balance.
--
-- Formula: gtq_reserved_cents = usd_cents * usd_gtq_rate / 100
-- Example: $10.00 USD (1000 cents) × 790 / 100 = 7 900 GTQ centavos (Q79.00)
--
-- Default: 790 (≈ Q7.90 per $1 USD — approximate market rate at launch).
-- is_runtime=TRUE: operators can update this via the admin system-params API
-- and the change propagates within the 30 s cache window without a restart.
-- Update whenever the exchange rate shifts significantly (>5 % from default).

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'payment.usd_gtq_rate',
    '790',
    '790',
    'int',
    'payment',
    TRUE,
    'GTQ centavos per USD dollar used for balance reservation on USD PayPal '
    'withdrawals (e.g. 790 = Q7.90 per $1.00). Formula: '
    'gtq_reserved_cents = usd_cents * rate / 100. Update when the exchange '
    'rate shifts significantly; changes propagate within 30 s (is_runtime=TRUE).'
)
ON CONFLICT (key) DO NOTHING;
