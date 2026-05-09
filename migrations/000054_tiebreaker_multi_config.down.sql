ALTER TABLE tiebreakers
    DROP CONSTRAINT uq_tiebreakers_user_config;

ALTER TABLE tiebreakers
    ADD CONSTRAINT uq_tiebreakers_user UNIQUE (user_id);

ALTER TABLE tiebreakers
    DROP COLUMN tiebreaker_config_id;

DROP INDEX uq_tiebreaker_config_quiniela_phase;
DROP INDEX uq_tiebreaker_config_quiniela;
DROP INDEX uq_tiebreaker_config_phase;
DROP INDEX uq_tiebreaker_config_global;

ALTER TABLE tiebreaker_config
    DROP COLUMN quiniela_id,
    DROP COLUMN phase;
