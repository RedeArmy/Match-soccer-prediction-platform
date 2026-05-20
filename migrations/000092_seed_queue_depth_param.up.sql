-- Seed the bank-transfer queue-depth alert threshold.
--
-- When the number of pending bank-transfer proofs reaches this value the
-- scheduler emits EventAdminBankTransferQueueDepth (P0) in addition to the
-- regular EventAdminPendingReminder.  Operators can lower the threshold for
-- faster alerting or raise it during known high-volume periods.
--
-- Idempotent: ON CONFLICT DO NOTHING is safe on re-run.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.bank_transfer_queue_depth_threshold',
    '20', '20',
    'int', 'notify',
    TRUE,
    'Number of pending bank-transfer proofs that triggers a P0 EventAdminBankTransferQueueDepth alert. Emitted alongside the regular pending reminder. Default: 20. Changeable at runtime.'
)
ON CONFLICT (key) DO NOTHING;
