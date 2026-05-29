-- Migration 000134: add scored_at to predictions for idempotent scoring.
--
-- scored_at is set the first time a prediction's points are written by
-- ScoreMatch. On re-delivery of a MatchFinished event (Redis Streams
-- at-least-once delivery), the idempotent UPDATE in UpdateManyPoints skips
-- predictions that already have scored_at set, preventing duplicate entries
-- in prediction_score_log and double-counting in leaderboard aggregations.
--
-- The partial index on (scored_at IS NULL) accelerates the WHERE scored_at IS NULL
-- filter in the idempotent scoring UPDATE. Unscored predictions are a tiny
-- fraction of the table during steady-state operation; the partial index keeps
-- the index small and the filter fast.

ALTER TABLE predictions
  ADD COLUMN IF NOT EXISTS scored_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_predictions_unscored
  ON predictions (id)
  WHERE scored_at IS NULL;
