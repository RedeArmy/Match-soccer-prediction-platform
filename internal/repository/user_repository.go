package repository

// TODO: implement PostgresUserRepository — the PostgreSQL-backed implementation
// of UserRepository defined in interfaces.go.
//
// Inject a *pgxpool.Pool at construction time. Each method must propagate
// the context.Context argument to pgxpool calls so that database connections
// are released promptly when the originating HTTP request is cancelled.
