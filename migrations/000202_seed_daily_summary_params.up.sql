-- Migration 000202: seed worker.sched_daily_summary_interval_sec and
-- worker.sched_daily_summary_result_delay_min system parameters.
--
-- interval_sec (900 = 15 min): how often the daily-summary job runs.
-- result_delay_min (30): how long after the last match result is entered before
-- the summary emails are dispatched — gives scorers time to finish all results.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('worker.sched_daily_summary_interval_sec', '900', '900', 'int', 'worker', TRUE,
     'How often (seconds) the daily-summary email scheduler job runs. Runtime: yes.'),
    ('worker.sched_daily_summary_result_delay_min', '30', '30', 'int', 'worker', TRUE,
     'Minutes to wait after all match results of the day are entered before sending daily summary emails. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;
