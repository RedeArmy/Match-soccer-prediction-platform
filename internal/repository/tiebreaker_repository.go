package repository

// TODO: implement PostgresTiebreakerRepository — the PostgreSQL-backed
// implementation of TiebreakerRepository defined in interfaces.go.
//
// Enforce a unique index on (user_id, quiniela_id) at the database level
// so that a single player cannot submit two tiebreaker forecasts for the
// same quiniela, even under concurrent requests from the same client.
