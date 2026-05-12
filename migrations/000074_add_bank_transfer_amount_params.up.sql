-- Seed system_params rows for bank transfer amount bounds.
--
-- The withdrawal service already enforces payment.withdrawal_min_cents and
-- payment.withdrawal_max_cents at the service layer. Bank transfer proofs lacked
-- equivalent bounds: the handler only validated amount_cents > 0, leaving no
-- guard against unreasonably small (1-cent) or arbitrarily large claims.
--
-- These params are operator-tunable at runtime (is_runtime = TRUE) so limits
-- can be adjusted during a tournament without a process restart.

INSERT INTO system_params (key, value, default_value, type, category, description, is_runtime)
VALUES
  (
    'payment.bank_transfer_min_amount_cents',
    '1000', '1000',
    'int', 'payment',
    'Minimum declared amount in minor currency units for bank transfer proof submissions (default 1000 = 10 GTQ)',
    TRUE
  ),
  (
    'payment.bank_transfer_max_amount_cents',
    '10000000', '10000000',
    'int', 'payment',
    'Maximum declared amount in minor currency units for bank transfer proof submissions (default 10000000 = 100000 GTQ)',
    TRUE
  )
ON CONFLICT (key) DO NOTHING;
