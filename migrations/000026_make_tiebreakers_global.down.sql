ALTER TABLE tiebreakers
    DROP CONSTRAINT IF EXISTS uq_tiebreakers_user,
    ADD COLUMN quiniela_id INTEGER REFERENCES quinielas(id) ON DELETE CASCADE,
    ADD COLUMN result      INTEGER,
    ADD CONSTRAINT uq_tiebreakers_user_quiniela UNIQUE (user_id, quiniela_id);
