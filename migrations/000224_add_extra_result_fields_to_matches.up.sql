-- Adds the fields needed to auto-resolve "extras" (bonus predictions): which
-- team scored first, and the half-time score. Both are populated by the
-- API-Football sync (see match_sync_service.go) once a match finishes;
-- admins can also set them manually via PATCH /matches/{id} or
-- /matches/{id}/correct-result when the provider data is missing or wrong.
--
-- All three columns are nullable and default to NULL, so matches finished
-- before this migration shipped simply render as "unavailable" in the extras
-- UI rather than showing a fabricated answer.
ALTER TABLE matches ADD COLUMN IF NOT EXISTS halftime_home_score INT;
ALTER TABLE matches ADD COLUMN IF NOT EXISTS halftime_away_score INT;
ALTER TABLE matches ADD COLUMN IF NOT EXISTS first_scoring_team TEXT
    CHECK (first_scoring_team IN ('home', 'away', 'none'));

COMMENT ON COLUMN matches.first_scoring_team IS
    '''none'' means the match finished 0-0; NULL means not yet resolved.';
