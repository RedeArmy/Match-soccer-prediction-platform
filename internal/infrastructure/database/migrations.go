package database

// TODO: implement schema migration support using golang-migrate/migrate.
//
// The Migrate function should accept a *pgxpool.Pool (or DSN string),
// locate the SQL migration files in the migrations/ directory, and apply
// any pending migrations in order. It should be called from cmd/migrate/main.go,
// not from the API server startup, to avoid race conditions when multiple
// replicas start simultaneously and each attempts to acquire the migration lock.
