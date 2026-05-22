-- Migration 000107: seed notification scheduler polling-interval system parameters.
--
-- The notification scheduler previously used hardcoded polling cadences for its
-- five interval-based jobs.  Promoting them to system_params allows operators to
-- tune job frequency via the admin API during the tournament without a worker
-- restart (NOTE: the new values take effect only after a worker restart because
-- all scheduler params are is_runtime=FALSE — the scheduler is wired once at
-- startup from the param values read at process launch).
--
-- Defaults match the previous hardcoded values so existing deployments are
-- unaffected by this migration.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    ('worker.sched_pred_deadline_interval_sec', '300', '300', 'int', 'worker', FALSE,
     'Seconds between prediction.deadline_approaching scheduler job fires (60–3600). Worker restart required to apply changes.'),
    ('worker.sched_match_result_interval_sec', '900', '900', 'int', 'worker', FALSE,
     'Seconds between admin.match_result_pending scheduler job fires (60–3600). Worker restart required to apply changes.'),
    ('worker.sched_pending_reminder_interval_sec', '14400', '14400', 'int', 'worker', FALSE,
     'Seconds between admin.pending_reminder scheduler job fires (60–86400). Worker restart required to apply changes.'),
    ('worker.sched_stale_escalation_interval_sec', '1800', '1800', 'int', 'worker', FALSE,
     'Seconds between admin.stale_escalation scheduler job fires (60–86400). Worker restart required to apply changes.'),
    ('worker.sched_push_prune_interval_sec', '86400', '86400', 'int', 'worker', FALSE,
     'Seconds between push.subscription_prune scheduler job fires (3600–604800). Worker restart required to apply changes.')
ON CONFLICT (key) DO NOTHING;
