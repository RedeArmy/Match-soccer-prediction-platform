-- Revert membership and prediction index optimizations.

DROP INDEX IF EXISTS idx_group_memberships_quiniela_status_paid;
DROP INDEX IF EXISTS idx_predictions_user_points;
