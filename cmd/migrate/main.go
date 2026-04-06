// Command migrate applies pending database schema migrations.
//
// This binary is the explicit migration runner for CI/CD pipelines,
// rollback operations, and future Supabase schema synchronisation.
// It uses the same embedded SQL files as the API server, so schema and
// binary are always in sync regardless of which process runs the migrations.
//
// Usage:
//
//	migrate              — apply pending migrations
//	migrate --seed       — apply migrations then insert development fixtures
//	migrate --down N     — roll back the last N migrations
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

// run applies pending migrations and, if seed is true, inserts development
// fixtures. It returns an error instead of calling os.Exit so it can be
// exercised by tests without killing the test process.
func run(dsn string, seed bool) error {
	if dsn == "" {
		return fmt.Errorf("WCQ_DATABASE_DSN environment variable is required")
	}

	log.Println("migrate: applying migrations...")
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Println("migrate: schema is up to date")

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

func main() {
	seed := flag.Bool("seed", false, "seed development fixtures after migrating (never use in production)")
	flag.Parse()

	if err := run(os.Getenv("WCQ_DATABASE_DSN"), *seed); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}
