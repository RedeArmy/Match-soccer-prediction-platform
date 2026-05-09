package database

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// baselineMigrationsTable is the golang-migrate schema history table.
const baselineMigrationsTable = "schema_migrations"

// migrationVersionRE matches the version prefix in migration file names,
// e.g. "000054" in "000054_tiebreaker_multi_config.up.sql".
var migrationVersionRE = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)

// ApplyBaseline executes baselineSQL against the database identified by pool.
// It is a raw DDL execution; no migration tracking is updated. Call
// MarkMigrationsApplied afterwards to tell golang-migrate that the versioned
// migrations covered by the baseline are already present.
func ApplyBaseline(ctx context.Context, pool *pgxpool.Pool, baselineSQL string) error {
	if _, err := pool.Exec(ctx, baselineSQL); err != nil {
		return fmt.Errorf("apply baseline: %w", err)
	}
	return nil
}

// MarkMigrationsApplied inserts version rows into the golang-migrate tracking
// table (schema_migrations) for every *.up.sql file found in sqlFS, marking
// them all as applied without re-executing their SQL.
//
// This is the second step of the fresh-environment bootstrap:
//
//  1. Apply the consolidated baseline DDL (ApplyBaseline).
//  2. Call MarkMigrationsApplied so golang-migrate considers the baseline
//     migrations as already run.
//  3. Start the application; Migrate() picks up only new migrations.
func MarkMigrationsApplied(ctx context.Context, pool *pgxpool.Pool, sqlFS fs.FS) error {
	versions, err := collectUpVersions(sqlFS)
	if err != nil {
		return fmt.Errorf("mark migrations: collect versions: %w", err)
	}
	if len(versions) == 0 {
		return nil
	}

	// Ensure the tracking table exists.
	_, err = pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL)`,
		baselineMigrationsTable,
	))
	if err != nil {
		return fmt.Errorf("mark migrations: ensure table: %w", err)
	}

	// golang-migrate's postgres driver stores exactly ONE row in schema_migrations
	// representing the highest applied version. Insert only the max version so
	// that on the next Migrate() call the driver correctly sees everything as
	// already applied and returns ErrNoChange.
	sort.Ints(versions)
	maxVersion := versions[len(versions)-1]
	_, err = pool.Exec(ctx,
		fmt.Sprintf(
			`INSERT INTO %s (version, dirty) VALUES ($1, false)
			 ON CONFLICT (version) DO UPDATE SET dirty = false`,
			baselineMigrationsTable,
		),
		maxVersion,
	)
	if err != nil {
		return fmt.Errorf("mark migrations: upsert version %d: %w", maxVersion, err)
	}
	return nil
}

// MigrateFresh is the fast-path bootstrap for new environments. It:
//
//  1. Reads the consolidated DDL from baselineFile (path on the local FS).
//  2. Applies it to the database via ApplyBaseline.
//  3. Marks all migrations in sqlFS as applied via MarkMigrationsApplied.
//
// After MigrateFresh returns, a subsequent call to Migrate() will be a no-op
// (no pending migrations) unless new migration files were added after the
// baseline was generated.
//
// baselineFile must be the path to migrations/baseline/schema.sql.
// sqlFS is typically the embed.FS from the migrations package.
func MigrateFresh(ctx context.Context, dsn string, baselineFile string, sqlFS fs.FS) error {
	baselineSQL, err := os.ReadFile(baselineFile)
	if err != nil {
		return fmt.Errorf("migrate fresh: read baseline %s: %w", baselineFile, err)
	}

	pool, err := NewPool(ctx, Config{
		DSN:             dsn,
		MaxOpenConns:    3,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		return fmt.Errorf("migrate fresh: connect: %w", err)
	}
	defer pool.Close()

	if err := ApplyBaseline(ctx, pool, string(baselineSQL)); err != nil {
		return err
	}
	return MarkMigrationsApplied(ctx, pool, sqlFS)
}

// collectUpVersions reads sqlFS and returns the integer version number of every
// *.up.sql file found in the root of the filesystem.
func collectUpVersions(sqlFS fs.FS) ([]int, error) {
	entries, err := fs.ReadDir(sqlFS, ".")
	if err != nil {
		return nil, err
	}
	var versions []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := filepath.Base(e.Name())
		m := migrationVersionRE.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		v, err := strconv.Atoi(strings.TrimLeft(m[1], "0"))
		if err != nil || v == 0 {
			continue
		}
		versions = append(versions, v)
	}
	return versions, nil
}
