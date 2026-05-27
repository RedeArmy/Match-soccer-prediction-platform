-- Migration 000121: seed KYC/AML system parameters.
--
-- All monetary limits are stored in centavos (1 GTQ = 100 centavos).
-- Tier limits align with Guatemalan SIB Regulation JM-47-2022 and the
-- UAF mandatory reporting threshold of Q25,000 per transaction.
--
-- All parameters are is_runtime=TRUE so that an admin can tighten or
-- relax limits via the admin API without restarting the worker process.
--
-- Active parameters (6 total):
--   tier1_deposit_limit_cents  — Q2,500  (Tier 0 and Tier 1 share this cap)
--   tier2_deposit_limit_cents  — Q15,000 (Tier 2 deposit per-transaction cap)
--   tier2_payout_limit_cents   — Q15,000 (Tier 2 withdrawal per-request cap)
--   aml_threshold_cents        — Q25,000 (UAF mandatory reporting threshold)
--   review_interval_days       — 365     (annual re-verification for Tier 2/3)
--   max_doc_upload_bytes       — 10 MB   (per-document upload size limit)
--
-- Removed params (business rule changes):
--   tier1_payout_limit_cents   — removed: Tier 1 is now fully blocked from
--                                withdrawals; a payout cap is not applicable.
--   win_freeze_threshold_cents — removed: all prizes freeze for Tier 0/1
--                                regardless of amount; no threshold needed.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    ('kyc.tier1_deposit_limit_cents', '250000', '250000', 'int', 'kyc', TRUE,
     'Per-transaction deposit cap (centavos) for Tier 0 and Tier 1 users. Q2,500 default per SIB JM-47-2022. Tier 1 (phone-verified) shares this limit with unverified users.'),

    ('kyc.tier2_deposit_limit_cents', '1500000', '1500000', 'int', 'kyc', TRUE,
     'Per-transaction deposit cap (centavos) for Tier 2 (gov ID + selfie) users. Q15,000 default per SIB JM-47-2022.'),

    ('kyc.tier2_payout_limit_cents', '1500000', '1500000', 'int', 'kyc', TRUE,
     'Per-request withdrawal cap (centavos) for Tier 2 users. Q15,000 default. Tier 3 is unlimited. Tiers 0 and 1 are blocked from withdrawals entirely.'),

    ('kyc.aml_threshold_cents', '2500000', '2500000', 'int', 'kyc', TRUE,
     'Transaction amount (centavos) above which a UAF AML report is mandatory under Guatemalan law. Deposits and withdrawals exceeding this value are flagged in the audit log. Q25,000 default.'),

    ('kyc.review_interval_days', '365', '365', 'int', 'kyc', TRUE,
     'Days before a Tier 2 or Tier 3 profile is due for re-verification. Written to kyc_profiles.next_review_at on each approval. Default: 365 (annual).'),

    ('kyc.max_doc_upload_bytes', '10485760', '10485760', 'int', 'kyc', TRUE,
     'Maximum size in bytes for a single KYC document upload. Enforced by KYCService.UploadDocument before the metadata row is written. Default: 10,485,760 (10 MB).')

ON CONFLICT (key) DO NOTHING;
