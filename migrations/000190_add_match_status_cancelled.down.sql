ALTER TABLE matches
    DROP CONSTRAINT matches_status_check,
    ADD  CONSTRAINT matches_status_check
         CHECK (status IN ('scheduled', 'live', 'finished'));
