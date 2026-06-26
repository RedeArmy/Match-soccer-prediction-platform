-- api-football uses the anglicised spelling "Turkey"; the matches table
-- stores the official FIFA name "Türkiye" adopted in 2022.
INSERT INTO team_name_aliases (canonical_name, provider_name)
VALUES ('Türkiye', 'Turkey')
ON CONFLICT (provider, provider_name) DO NOTHING;
