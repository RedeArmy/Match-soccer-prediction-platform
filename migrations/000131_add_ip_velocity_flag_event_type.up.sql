-- Migration 000131: add ip_velocity_flag to kyc_events.event_type CHECK constraint.
--
-- KYCEventIPVelocityFlag ("ip_velocity_flag") was defined in Go but absent from
-- the database constraint. Every IP-velocity-triggered appendEvent call produced
-- a check_violation that was silently swallowed, leaving a gap in the compliance
-- audit trail for every blocked submission.

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
    'unfrozen',
    'ip_velocity_flag'
  ));
