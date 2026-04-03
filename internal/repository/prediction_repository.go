package repository

// TODO: implement PostgresPredictionRepository — the PostgreSQL-backed
// implementation of PredictionRepository defined in interfaces.go.
//
// GetByUserAndMatch must use a unique index on (user_id, match_id) to
// ensure the database enforces the one-prediction-per-user-per-match
// invariant independently of the application layer, eliminating the
// check-then-act race condition under concurrent requests.
