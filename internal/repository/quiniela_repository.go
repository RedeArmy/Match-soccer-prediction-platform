package repository

// TODO: implement PostgresQuinielaRepository — the PostgreSQL-backed
// implementation of QuinielaRepository defined in interfaces.go.
//
// GetByID should NOT hydrate the Predictions slice — that is the service
// layer's responsibility when a full detail view is required. Returning
// a lightweight Quiniela (metadata only) keeps list queries fast and
// avoids loading thousands of predictions on every dashboard render.
