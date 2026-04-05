package database

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx5 driver for golang-migrate
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies all pending UP migrations from the provided fs.FS.
//
// The FS is typically the embed.FS exported by the migrations package, which
// embeds the SQL files at compile time. Using an embedded FS means the binary
// is self-contained: no SQL files need to be present on the filesystem at
// runtime, and deployments cannot drift from the version that was compiled.
//
// golang-migrate acquires a PostgreSQL advisory lock before applying any
// migration, so concurrent calls (e.g. two API replicas starting at the same
// time) are safe: one runner holds the lock and applies pending migrations
// while the other waits and then finds no further changes.
//
// A "no change" result (migrate.ErrNoChange) is treated as success because
// the runner is expected to be idempotent.
func Migrate(dsn string, sqlFS fs.FS) error {
	source, err := iofs.New(sqlFS, ".")
	if err != nil {
		return fmt.Errorf("migrate: load source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, toPGX5DSN(dsn))
	if err != nil {
		return fmt.Errorf("migrate: initialise: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}

// toPGX5DSN rewrites a standard PostgreSQL DSN to use the pgx5:// scheme
// expected by the golang-migrate pgx/v5 driver.
func toPGX5DSN(dsn string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(dsn, prefix) {
			return "pgx5://" + strings.TrimPrefix(dsn, prefix)
		}
	}
	return dsn
}
