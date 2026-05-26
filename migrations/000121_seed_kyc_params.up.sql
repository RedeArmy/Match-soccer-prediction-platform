-- Migration 000121: seed KYC/AML system parameters.
--
-- All monetary limits are stored in centavos (1 GTQ = 100 centavos).
-- Tier limits align with Guatemalan SIB Regulation JM-47-2022 and the
-- UAF mandatory reporting threshold of Q25,000 per transaction or
-- Q50,000 aggregate per month.
--
-- All parameters are is_runtime=TRUE so that an admin can tighten or
-- relax limits via the admin API without restarting the worker process.
--
-- Defaults:
--   Tier 1 (phone verified)   — Q2,500/month deposit, Q2,500 per payout
--   Tier 2 (gov ID + selfie)  — Q15,000/month deposit, Q15,000 per payout
--   Tier 3 (full KYC)         — unlimited (enforced by removing the gate)
--   win_freeze_threshold      — Q5,000 (freeze balance for Tier 0/1 users)
--   aml_threshold             — Q25,000 (mandatory UAF reporting)
--   review_interval_days      — 365 (annual re-verification for Tier 2/3)
--   max_doc_upload_bytes      — 10,485,760 (10 MB per document)

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    ('kyc.tier1_deposit_limit_cents', '250000', '250000', 'int', 'kyc', TRUE,
     'Monthly deposit cap (centavos) for Tier 1 (phone-verified) users. Q2,500 default per SIB JM-47-2022.'),

    ('kyc.tier1_payout_limit_cents', '250000', '250000', 'int', 'kyc', TRUE,
     'Per-request withdrawal cap (centavos) for Tier 1 users. Q2,500 default.'),

    ('kyc.tier2_deposit_limit_cents', '1500000', '1500000', 'int', 'kyc', TRUE,
     'Monthly deposit cap (centavos) for Tier 2 (gov ID + selfie) users. Q15,000 default per SIB JM-47-2022.'),

    ('kyc.tier2_payout_limit_cents', '1500000', '1500000', 'int', 'kyc', TRUE,
     'Per-request withdrawal cap (centavos) for Tier 2 users. Q15,000 default.'),

    ('kyc.win_freeze_threshold_cents', '500000', '500000', 'int', 'kyc', TRUE,
     'Prize credit (centavos) that triggers a balance freeze for Tier 0/1 users pending KYC. Q5,000 default.'),

    ('kyc.aml_threshold_cents', '2500000', '2500000', 'int', 'kyc', TRUE,
     'Transaction amount (centavos) above which a UAF report is mandatory under Guatemalan AML law. Q25,000 default.'),

    ('kyc.review_interval_days', '365', '365', 'int', 'kyc', TRUE,
     'Days between mandatory re-verification checks for Tier 2 and Tier 3 profiles.'),

    ('kyc.max_doc_upload_bytes', '10485760', '10485760', 'int', 'kyc', TRUE,
     'Maximum size in bytes for a single KYC document upload. Default 10 MB.')
ON CONFLICT (key) DO NOTHING;
