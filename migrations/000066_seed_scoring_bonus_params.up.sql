-- Migration: seed scoring.extra_time_bonus and scoring.penalties_bonus system parameters
--
-- These two keys were introduced in domain/constants.go alongside the phase-aware
-- scoring feature (migration 000065 / feat/phase-aware-scoring). The scoring service
-- reads them as the global baseline fallback when no per-phase ScoringRule row is
-- active for a given match:
--
--   scoring_service.go:
--     extraTimeBonus: s.params.GetInt(ctx, domain.ParamKeyScoringExtraTimeBonus, 0)
--     penaltiesBonus: s.params.GetInt(ctx, domain.ParamKeyScoringPenaltiesBonus, 0)
--
-- The per-phase scoring_rules table (migration 000063) already seeds non-zero bonus
-- values for all knockout phases (extra_time_bonus=1, penalties_bonus=2 in 000065),
-- so the global fallback defaults to 0 — which means "no bonus unless a phase rule
-- overrides it". Operators can raise the global fallback without touching per-phase
-- rules, e.g. to apply a blanket bonus during a tournament restart.
--
-- Both params are is_runtime=TRUE: changes take effect within the cache window
-- (30 s for runtime params) without a process restart.
--
-- default_value is required (NOT NULL since migration 000060).
-- ON CONFLICT updates metadata only, never the operator-set value, so this migration
-- is idempotent and safe to re-run on environments that already have these rows.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES

    ('scoring.extra_time_bonus', '0', '0', 'int', 'scoring', TRUE,
     'Global fallback bonus points added when a knockout prediction is correct and the match was settled via extra time. Overridden per-phase by scoring_rules. 0 = no global bonus.'),

    ('scoring.penalties_bonus',  '0', '0', 'int', 'scoring', TRUE,
     'Global fallback bonus points added when a knockout prediction is correct and the match was settled via penalty shootout. Overridden per-phase by scoring_rules. 0 = no global bonus.')

ON CONFLICT (key) DO UPDATE SET
    default_value = EXCLUDED.default_value,
    type          = EXCLUDED.type,
    category      = EXCLUDED.category,
    is_runtime    = EXCLUDED.is_runtime,
    description   = EXCLUDED.description,
    updated_at    = NOW();
-- value is intentionally NOT updated so that operator overrides survive re-migrations.
