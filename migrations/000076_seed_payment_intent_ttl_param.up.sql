-- Migration: seed payment.intent_ttl_minutes system parameter
--
-- Adds the runtime-configurable TTL for PayPal payment intents. After this
-- window elapses, a pending intent can no longer be captured by a webhook
-- and the customer must restart the checkout flow.
--
-- The value is read by PaymentIntentService.Create via the SystemParamService
-- on every call (is_runtime=TRUE), so operators can shorten or extend the
-- window without a process restart.
--
-- 60 minutes covers a typical PayPal checkout session comfortably. Lowering
-- the value increases security (smaller stale-capture window); raising it
-- improves UX for slow connections.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'payment.intent_ttl_minutes',
    '60', '60',
    'int', 'payment',
    TRUE,
    'Minutes a pending PayPal payment intent remains valid before expiry. After this window the webhook returns NotFound and the customer must restart checkout. Tunable at runtime without a process restart.'
)
ON CONFLICT (key) DO NOTHING;
