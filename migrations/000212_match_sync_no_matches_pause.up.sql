-- Migration 000212: system param for the configurable no-matches polling pause.
-- When the match-sync job finds no upcoming linked matches it pauses polling for
-- this many seconds before re-checking the DB. Keeping it in system_params lets
-- operators shorten the window (e.g. after manually linking a new match) without
-- a process restart. is_runtime=true.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('match.sync.no_matches_pause_sec',
     '300', '300', 'int', 'match', true,
     'Seconds the match-sync polling job waits before re-checking the DB when no upcoming linked matches are found. During this window, a match newly linked via external_match_id is picked up on the next re-check rather than requiring a process restart. Defaults to 300 (5 min).')
ON CONFLICT (key) DO NOTHING;
