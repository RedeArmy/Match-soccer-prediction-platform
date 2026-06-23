DELETE FROM system_params WHERE key IN (
    'worker.sched_daily_summary_interval_sec',
    'worker.sched_daily_summary_result_delay_min'
);
