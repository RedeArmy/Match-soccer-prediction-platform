-- Extend the match status check constraint to allow 'cancelled'.
-- Cancelled matches are set manually by admins for postponed or abandoned fixtures.
ALTER TABLE matches
    DROP CONSTRAINT matches_status_check,
    ADD  CONSTRAINT matches_status_check
         CHECK (status IN ('scheduled', 'live', 'finished', 'cancelled'));
