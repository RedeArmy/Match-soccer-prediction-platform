// Command migrate applies pending database schema migrations.
//
// This binary is the explicit migration runner for CI/CD pipelines,
// rollback operations, and new-environment bootstrapping.
// It uses the same embedded SQL files as the API server, so schema and
// binary are always in sync regardless of which process runs the migrations.
//
// Usage:
//
//	migrate                        - apply pending migrations (normal path)
//	migrate --seed                 - apply migrations then insert development fixtures
//	migrate --fresh                - bootstrap from baseline (new environments only)
//	migrate --fresh --seed         - baseline + development fixtures
//	migrate --fresh --baseline=PATH - use a custom baseline file
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/migrations"
)

const defaultBaselinePath = "migrations/baseline/schema.sql"

// run applies pending migrations and, if seed is true, inserts development
// fixtures. When fresh is true it applies the consolidated baseline DDL and
// marks all versioned migrations as applied, bypassing sequential execution.
// It returns an error instead of calling os.Exit so it can be exercised by
// tests without killing the test process.
func run(dsn string, seed, fresh bool, baselinePath string) error {
	if dsn == "" {
		return fmt.Errorf("WCQ_DATABASE_DSN environment variable is required")
	}

	if fresh {
		log.Printf("migrate: applying baseline from %s ...", baselinePath)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := database.MigrateFresh(ctx, dsn, baselinePath, migrations.FS); err != nil {
			return fmt.Errorf("migrate fresh: %w", err)
		}
		log.Println("migrate: baseline applied; all migrations marked as applied")
	} else {
		log.Println("migrate: applying pending migrations...")
		if err := database.Migrate(dsn, migrations.FS); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		log.Println("migrate: schema is up to date")
	}

	if seed {
		log.Println("migrate: seeding development fixtures...")
		cfg := database.Config{
			DSN:             dsn,
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: 5 * time.Minute,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool, err := database.NewPool(ctx, cfg)
		if err != nil {
			return fmt.Errorf("migrate: connect for seed: %w", err)
		}
		defer pool.Close()

		if err := database.Seed(ctx, pool); err != nil {
			return fmt.Errorf("migrate: seed: %w", err)
		}
		log.Println("migrate: seed complete")
	}
	return nil
}

// osExit is a package-level variable so tests can replace it with a function
// that records the exit code instead of terminating the process.
var osExit = os.Exit

func main() {
	seed := flag.Bool("seed", false, "seed development fixtures after migrating (never use in production)")
	fresh := flag.Bool("fresh", false, "bootstrap from consolidated baseline DDL (new environments only — skips running all 67+ migrations sequentially)")
	baseline := flag.String("baseline", defaultBaselinePath, "path to baseline SQL file (used only with --fresh)")
	flag.Parse()

	if err := run(os.Getenv("WCQ_DATABASE_DSN"), *seed, *fresh, *baseline); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		osExit(1)
	}
}
