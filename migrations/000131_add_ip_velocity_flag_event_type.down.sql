ALTER TABLE kyc_events DROP CONSTRAINT kyc_events_event_type_check;

ALTER TABLE kyc_events ADD CONSTRAINT kyc_events_event_type_check
  CHECK (event_type IN (
    'submitted',
    'under_review',
    'approved',
    'rejected',
    'escalated',
    'expired',
    'tier_changed',
    'doc_requested',
    'frozen',
    'unfrozen'
  ));
