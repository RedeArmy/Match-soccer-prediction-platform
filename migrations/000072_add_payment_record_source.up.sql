-- Record which channel originated a payment record so the admin UI can display
-- the source alongside the payment status.
--
-- manual            - created by an admin directly (legacy default)
-- webhook_recurrente - auto-created by the Recurrente payment webhook
-- webhook_paypal     - auto-created by the PayPal payment webhook

ALTER TABLE payment_records
  ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'
    CHECK (source IN ('manual', 'webhook_recurrente', 'webhook_paypal'));
