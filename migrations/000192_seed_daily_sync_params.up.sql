-- Migration 000192: system params for the daily fixture sync job,
-- pre-match polling window, and consecutive-zero-live stop threshold.
-- All params are runtime-tunable (is_runtime=true).
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('match.dailysync.hour',
     '1', '1', 'int', 'match', true,
     'Local hour (0-23, America/Guatemala) at which the daily fixture sync job runs once to validate all linked scheduled and live matches against API-Football. The job fires exactly once per calendar day at this hour; failures trigger an n8n admin alert and are not retried until the next day.'),
    ('match.sync.prematch_window_min',
     '10', '10', 'int', 'match', true,
     'Minutes before kick-off after which the match-sync polling job begins calling API-Football for a scheduled match. Scheduled matches with a kickoff more than this many minutes in the future are skipped to conserve API quota. Live matches are always polled regardless of this value. Also used as the resume threshold when polling is paused after all matches finish.'),
    ('match.sync.stop_after_zero_live_count',
     '3', '3', 'int', 'match', true,
     'Number of consecutive poll cycles that return zero live matches before the polling job suspends API calls. Polling resumes automatically when within match.sync.prematch_window_min minutes of the next scheduled match kickoff.')
ON CONFLICT (key) DO NOTHING;
