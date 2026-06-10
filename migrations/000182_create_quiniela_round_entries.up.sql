-- Migration 000182: create quiniela_round_entries table.
--
-- Tracks which members have paid to participate in a specific round/jornada of
-- a per-round-mode tournament. A row here is required for a member to be
-- included in that round's prize distribution.
--
-- round_key matches the round_number cast to text from the matches table:
--   "1" → group-stage matchday 1
--   "2" → group-stage matchday 2
--   "3" → group-stage matchday 3
--   "4" → round_of_32
--   "5" → round_of_16
--   "6" → quarter_final
--   "7" → semi_final
--   "8" → third_place
--   "9" → final
--
-- The unique constraint on (quiniela_id, user_id, round_key) ensures each
-- member can only pay once per round per group (idempotent registration).

CREATE TABLE quiniela_round_entries (
    id          SERIAL PRIMARY KEY,
    quiniela_id INT          NOT NULL REFERENCES quinielas(id) ON DELETE CASCADE,
    user_id     INT          NOT NULL REFERENCES users(id)     ON DELETE CASCADE,
    round_key   VARCHAR(10)  NOT NULL,
    paid_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_quiniela_round_user UNIQUE (quiniela_id, user_id, round_key)
);

CREATE INDEX idx_qre_quiniela_round ON quiniela_round_entries (quiniela_id, round_key);
