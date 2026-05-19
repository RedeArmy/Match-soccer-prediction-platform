DELETE FROM system_params
WHERE key IN (
    'notify.bank_transfer_stale_sec',
    'notify.withdrawal_stale_sec',
    'notify.high_value_withdrawal_cents',
    'notify.pending_reminder_interval_sec',
    'notify.prediction_deadline_lead_min_1',
    'notify.prediction_deadline_lead_min_2',
    'notify.prediction_missing_lead_min',
    'notify.admin_emails',
    'notify.web_push_vapid_public_key',
    'notify.web_push_vapid_private_key',
    'notify.web_push_vapid_subject'
);
