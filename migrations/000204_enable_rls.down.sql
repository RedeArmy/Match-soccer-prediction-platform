-- Disable Row-Level Security on all tables (rolls back migration 000204).
-- WARNING: executing this migration re-exposes all tables through the
-- Supabase PostgREST API without any access restrictions.

-- ── Core domain ──────────────────────────────────────────────────────────────
ALTER TABLE users                     DISABLE ROW LEVEL SECURITY;
ALTER TABLE quinielas                 DISABLE ROW LEVEL SECURITY;
ALTER TABLE group_memberships         DISABLE ROW LEVEL SECURITY;
ALTER TABLE quiniela_round_entries    DISABLE ROW LEVEL SECURITY;

-- ── Tournament & fixtures ─────────────────────────────────────────────────────
ALTER TABLE matches                   DISABLE ROW LEVEL SECURITY;
ALTER TABLE stadiums                  DISABLE ROW LEVEL SECURITY;
ALTER TABLE teams                     DISABLE ROW LEVEL SECURITY;
ALTER TABLE team_name_aliases         DISABLE ROW LEVEL SECURITY;
ALTER TABLE tournament_slots          DISABLE ROW LEVEL SECURITY;
ALTER TABLE scoring_rules             DISABLE ROW LEVEL SECURITY;
ALTER TABLE tiebreaker_config         DISABLE ROW LEVEL SECURITY;
ALTER TABLE tiebreakers               DISABLE ROW LEVEL SECURITY;

-- ── Predictions & scoring ─────────────────────────────────────────────────────
ALTER TABLE predictions               DISABLE ROW LEVEL SECURITY;
ALTER TABLE prediction_score_log      DISABLE ROW LEVEL SECURITY;
ALTER TABLE leaderboard_snapshots     DISABLE ROW LEVEL SECURITY;

-- ── Payments & financials ─────────────────────────────────────────────────────
ALTER TABLE payment_intents           DISABLE ROW LEVEL SECURITY;
ALTER TABLE payment_records           DISABLE ROW LEVEL SECURITY;
ALTER TABLE bank_transfer_proofs      DISABLE ROW LEVEL SECURITY;
ALTER TABLE withdrawal_requests       DISABLE ROW LEVEL SECURITY;
ALTER TABLE balance_ledger            DISABLE ROW LEVEL SECURITY;
ALTER TABLE exchange_rate_history     DISABLE ROW LEVEL SECURITY;

-- ── KYC / compliance ──────────────────────────────────────────────────────────
ALTER TABLE kyc_profiles              DISABLE ROW LEVEL SECURITY;
ALTER TABLE kyc_documents             DISABLE ROW LEVEL SECURITY;
ALTER TABLE kyc_events                DISABLE ROW LEVEL SECURITY;

-- ── Notifications ─────────────────────────────────────────────────────────────
ALTER TABLE notifications             DISABLE ROW LEVEL SECURITY;
ALTER TABLE notification_preferences  DISABLE ROW LEVEL SECURITY;
ALTER TABLE notification_templates    DISABLE ROW LEVEL SECURITY;
ALTER TABLE notification_template_history DISABLE ROW LEVEL SECURITY;
ALTER TABLE admin_notification_log    DISABLE ROW LEVEL SECURITY;
ALTER TABLE push_subscriptions        DISABLE ROW LEVEL SECURITY;

-- ── Internal infrastructure ───────────────────────────────────────────────────
ALTER TABLE domain_outbox             DISABLE ROW LEVEL SECURITY;
ALTER TABLE notification_dlq          DISABLE ROW LEVEL SECURITY;
ALTER TABLE event_dlq                 DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log                 DISABLE ROW LEVEL SECURITY;

-- ── Reference / lookup data ───────────────────────────────────────────────────
ALTER TABLE countries                 DISABLE ROW LEVEL SECURITY;
ALTER TABLE states                    DISABLE ROW LEVEL SECURITY;
ALTER TABLE cities                    DISABLE ROW LEVEL SECURITY;
ALTER TABLE gt_banks                  DISABLE ROW LEVEL SECURITY;
ALTER TABLE bank_account_types        DISABLE ROW LEVEL SECURITY;

-- ── System configuration ──────────────────────────────────────────────────────
ALTER TABLE system_params             DISABLE ROW LEVEL SECURITY;
ALTER TABLE system_params_history     DISABLE ROW LEVEL SECURITY;
