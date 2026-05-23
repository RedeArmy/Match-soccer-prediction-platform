DELETE FROM system_params WHERE key IN (
    'worker.sched_pred_deadline_interval_sec',
    'worker.sched_match_result_interval_sec',
    'worker.sched_pending_reminder_interval_sec',
    'worker.sched_stale_escalation_interval_sec',
    'worker.sched_push_prune_interval_sec'
);
