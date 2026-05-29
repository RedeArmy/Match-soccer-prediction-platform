DROP INDEX IF EXISTS idx_predictions_unscored;
ALTER TABLE predictions DROP COLUMN IF EXISTS scored_at;
