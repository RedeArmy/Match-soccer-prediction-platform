-- Migration 000186: system params for the automated match-sync worker.
-- All params are runtime-tunable (is_runtime=true) so operators can adjust
-- polling intervals without a redeployment.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('match.sync.enabled',
     'false', 'false', 'bool', 'match', true,
     'When true the match-sync worker polls API-Football for live scores and automatically transitions match status and records results.'),
    ('match.sync.fast_poll_interval_sec',
     '30', '30', 'int', 'match', true,
     'Polling interval in seconds used while at least one match is live. Lower values increase data freshness but consume more API quota.'),
    ('match.sync.slow_poll_interval_sec',
     '300', '300', 'int', 'match', true,
     'Polling interval in seconds used when no matches are currently live. Reduces API quota consumption during off-hours.'),
    ('match.sync.provider',
     'api-football', 'api-football', 'string', 'match', true,
     'Identifier of the external data provider used by the sync worker. Currently only ''api-football'' is supported.'),
    ('match.sync.league_id',
     '1', '1', 'int', 'match', true,
     'API-Football league ID for the FIFA World Cup. Value 1 is the FIFA World Cup competition.'),
    ('match.sync.season',
     '2026', '2026', 'int', 'match', true,
     'Tournament season year used when querying the provider API (e.g. 2026).')
ON CONFLICT (key) DO NOTHING;
