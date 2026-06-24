-- Enable Row-Level Security on every table in the public schema.
--
-- WHY: Supabase exposes all public tables through its PostgREST API under the
-- anon / authenticated roles. With RLS disabled, anyone who holds the project
-- URL can read, modify, or delete data without authentication.
--
-- IMPACT ON THE APPLICATION: None. The Go backend connects as the postgres
-- superuser (or a role with BYPASSRLS), which is exempt from RLS checks.
-- No application code changes are required.
--
-- HOW IT WORKS: Enabling RLS with no permissive policy in place produces a
-- default-deny for all non-owner roles. The explicit REVOKEs below add a
-- belt-and-suspenders layer in case any future Supabase behaviour grants
-- table privileges to those roles again.

-- ── Core domain ──────────────────────────────────────────────────────────────
ALTER TABLE users                     ENABLE ROW LEVEL SECURITY;
ALTER TABLE quinielas                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_memberships         ENABLE ROW LEVEL SECURITY;
ALTER TABLE quiniela_round_entries    ENABLE ROW LEVEL SECURITY;

-- ── Tournament & fixtures ─────────────────────────────────────────────────────
ALTER TABLE matches                   ENABLE ROW LEVEL SECURITY;
ALTER TABLE stadiums                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams                     ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_name_aliases         ENABLE ROW LEVEL SECURITY;
ALTER TABLE tournament_slots          ENABLE ROW LEVEL SECURITY;
ALTER TABLE scoring_rules             ENABLE ROW LEVEL SECURITY;
ALTER TABLE tiebreaker_config         ENABLE ROW LEVEL SECURITY;
ALTER TABLE tiebreakers               ENABLE ROW LEVEL SECURITY;

-- ── Predictions & scoring ─────────────────────────────────────────────────────
ALTER TABLE predictions               ENABLE ROW LEVEL SECURITY;
ALTER TABLE prediction_score_log      ENABLE ROW LEVEL SECURITY;
ALTER TABLE leaderboard_snapshots     ENABLE ROW LEVEL SECURITY;

-- ── Payments & financials (sensitive) ─────────────────────────────────────────
ALTER TABLE payment_intents           ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_records           ENABLE ROW LEVEL SECURITY;
ALTER TABLE bank_transfer_proofs      ENABLE ROW LEVEL SECURITY;
ALTER TABLE withdrawal_requests       ENABLE ROW LEVEL SECURITY;
ALTER TABLE balance_ledger            ENABLE ROW LEVEL SECURITY;
ALTER TABLE exchange_rate_history     ENABLE ROW LEVEL SECURITY;

-- ── KYC / compliance (highly sensitive) ───────────────────────────────────────
ALTER TABLE kyc_profiles              ENABLE ROW LEVEL SECURITY;
ALTER TABLE kyc_documents             ENABLE ROW LEVEL SECURITY;
ALTER TABLE kyc_events                ENABLE ROW LEVEL SECURITY;

-- ── Notifications ─────────────────────────────────────────────────────────────
ALTER TABLE notifications             ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_preferences  ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_templates    ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_template_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE admin_notification_log    ENABLE ROW LEVEL SECURITY;
ALTER TABLE push_subscriptions        ENABLE ROW LEVEL SECURITY;

-- ── Internal infrastructure ───────────────────────────────────────────────────
ALTER TABLE domain_outbox             ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_dlq          ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_dlq                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log                 ENABLE ROW LEVEL SECURITY;

-- ── Reference / lookup data ───────────────────────────────────────────────────
ALTER TABLE countries                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE states                    ENABLE ROW LEVEL SECURITY;
ALTER TABLE cities                    ENABLE ROW LEVEL SECURITY;
ALTER TABLE gt_banks                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE bank_account_types        ENABLE ROW LEVEL SECURITY;

-- ── System configuration ──────────────────────────────────────────────────────
ALTER TABLE system_params             ENABLE ROW LEVEL SECURITY;
ALTER TABLE system_params_history     ENABLE ROW LEVEL SECURITY;

-- ── Belt-and-suspenders: revoke PostgREST role privileges ─────────────────────
-- The DO block guards against environments where these Supabase-managed roles
-- do not exist (e.g., plain Postgres CI containers).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM anon;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM authenticated;
    END IF;
END
$$;
