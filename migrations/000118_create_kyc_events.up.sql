-- Migration 000118: create kyc_events audit table.
--
-- Immutable append-only log of every KYC state-machine transition.
-- Rows are never updated or deleted; they form the complete audit trail
-- required for regulatory review.
--
-- actor_id is NULL for system-generated events (automatic expiry, win-freeze).
-- trace_id links the event to the distributed OTel trace that caused it,
-- enabling correlation between application traces and compliance records.
--
-- old_status is NULL on the first event for a profile (initial submission).
-- metadata carries any extra context: rejection reasons, document types
-- requested, freeze amounts, third-party provider result IDs, etc.

CREATE TABLE kyc_events (
  id           BIGSERIAL    PRIMARY KEY,
  profile_id   INT          NOT NULL,
  profile_type TEXT         NOT NULL DEFAULT 'user'
                 CHECK (profile_type IN ('user','org')),
  event_type   TEXT         NOT NULL
                 CHECK (event_type IN (
                   'submitted','under_review','approved','rejected',
                   'escalated','expired','tier_changed','doc_requested',
                   'frozen','unfrozen'
                 )),
  actor_id     INT          REFERENCES users(id) ON DELETE SET NULL,
  old_status   TEXT,
  new_status   TEXT         NOT NULL,
  reason       TEXT         NOT NULL DEFAULT '',
  metadata     JSONB        NOT NULL DEFAULT '{}',
  trace_id     TEXT         NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kyc_events_profile    ON kyc_events (profile_id, profile_type, created_at DESC);
CREATE INDEX idx_kyc_events_actor      ON kyc_events (actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX idx_kyc_events_event_type ON kyc_events (event_type, created_at DESC);
