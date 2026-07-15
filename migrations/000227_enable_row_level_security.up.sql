-- Enables Row-Level Security on every table in the public schema.
--
-- This project is hosted on Supabase, which auto-generates a public
-- PostgREST API (https://<project-ref>.supabase.co/rest/v1/<table>) for
-- every table, reachable with the project's anon key regardless of whether
-- the application actually uses it. This backend never uses Supabase's
-- client SDK, anon key, or PostgREST API — the only access path is this Go
-- service connecting via the `postgres` role over the Session-mode pooler
-- (see server/.env.example). PostgreSQL never applies row-level security to
-- the table owner or a superuser role, so enabling RLS here has zero effect
-- on the application; it exists purely to close the PostgREST exposure by
-- denying every request from the `anon` / `authenticated` roles PostgREST
-- connects as, since no policies are defined (RLS enabled + no policies =
-- deny all for every non-owner role).
--
-- No down-time, no application changes required.
ALTER TABLE public.admin_notification_log        ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.audit_log                      ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.balance_ledger                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bank_account_types             ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.bank_transfer_proofs           ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.cities                         ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.countries                      ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.domain_outbox                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.event_dlq                      ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.exchange_rate_history          ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.extra_predictions              ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.extra_rules                    ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.group_memberships              ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.gt_banks                       ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.kyc_documents                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.kyc_events                     ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.kyc_profiles                   ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.leaderboard_snapshots          ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.matches                        ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.notification_dlq               ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.notification_preferences       ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.notification_template_history  ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.notification_templates         ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.notifications                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.payment_intents                ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.payment_records                ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.prediction_score_log           ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.predictions                    ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.push_subscriptions             ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.quiniela_round_entries         ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.quinielas                      ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.revoked_sessions                ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.schema_migrations              ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.scoring_rules                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.session_starts                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.stadiums                       ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.states                         ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.system_params                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.system_params_history          ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.team_name_aliases              ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.teams                          ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.tiebreaker_config              ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.tiebreakers                    ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.tournament_slots               ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.users                          ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.withdrawal_requests            ENABLE ROW LEVEL SECURITY;
