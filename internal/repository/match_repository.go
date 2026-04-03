package repository

// TODO: implement PostgresMatchRepository — the PostgreSQL-backed implementation
// of MatchRepository defined in interfaces.go.
//
// ListByStatus maps directly to a WHERE status = $1 query. Ensure the
// matches table has an index on the status column before this goes to production;
// a full-table scan on every request for scheduled matches will degrade under load.
