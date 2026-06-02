-- Migration 000151: seed payment.intent_max_cents system parameter.
--
-- Adds an operator-tunable upper bound on PayPal payment intent amounts.
-- Without this cap any authenticated user could create a PayPal order for an
-- arbitrary amount; if the order were somehow captured, the webhook credit
-- would execute at the same inflated value.
--
-- Default: 50 000 centavos = Q500.00 — a conservative starting point suitable
-- for a sports-betting quiniela. Operators can raise this via the admin API
-- (PUT /admin/system-params/payment.intent_max_cents) without a restart
-- because is_runtime=TRUE.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'payment.intent_max_cents',
    '50000',
    '50000',
    'int',
    'payment',
    TRUE,
    'Maximum amount in GTQ minor units (centavos) for a PayPal payment intent. '
    'Requests above this value are rejected with 422 before a PayPal order is '
    'created, preventing absurdly large orders. Default: 50 000 (Q500.00). '
    'Valid range: 1–10 000 000.'
)
ON CONFLICT (key) DO NOTHING;
