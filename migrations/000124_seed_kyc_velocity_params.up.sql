-- Migration 000124: seed KYC velocity (24-hour rolling) limit parameters.
--
-- Velocity limits cap the total amount a user can deposit or withdraw in a
-- rolling 24-hour window. They complement the per-transaction caps seeded in
-- migration 000121 and are enforced by KYCGate.CheckDepositVelocity /
-- CheckWithdrawalVelocity, which sum balance_ledger rows via
-- SumTransactionsByUserAndPeriod.
--
-- All monetary values are in centavos (1 GTQ = 100 centavos).
-- All parameters are is_runtime=TRUE so an admin can adjust them without
-- restarting the worker process.
--
-- Defaults (align with constants_kyc.go Default* values):
--   tier1_deposit_velocity_cents    — Q50,000/24h   (Tier 0 and Tier 1 share)
--   tier2_deposit_velocity_cents    — Q200,000/24h  (Tier 2 users)
--   tier1_withdrawal_velocity_cents — Q0/24h        (Tier 0/1 blocked entirely)
--   tier2_withdrawal_velocity_cents — Q100,000/24h  (Tier 2 users)

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    ('kyc.tier1_deposit_velocity_cents', '5000000', '5000000', 'int', 'kyc', TRUE,
     'Maximum total deposits (centavos) allowed in a rolling 24-hour window for Tier 0 and Tier 1 users. Default: Q50,000 (5,000,000 centavos).'),

    ('kyc.tier2_deposit_velocity_cents', '20000000', '20000000', 'int', 'kyc', TRUE,
     'Maximum total deposits (centavos) allowed in a rolling 24-hour window for Tier 2 users. Default: Q200,000 (20,000,000 centavos).'),

    ('kyc.tier1_withdrawal_velocity_cents', '0', '0', 'int', 'kyc', TRUE,
     'Maximum total withdrawals (centavos) in 24h for Tier 0/1 users. 0 = fully blocked. Tier 0 and Tier 1 cannot withdraw; this param is a safety backstop.'),

    ('kyc.tier2_withdrawal_velocity_cents', '10000000', '10000000', 'int', 'kyc', TRUE,
     'Maximum total withdrawals (centavos) allowed in a rolling 24-hour window for Tier 2 users. Default: Q100,000 (10,000,000 centavos). Tier 3 is unlimited.')

ON CONFLICT (key) DO NOTHING;
