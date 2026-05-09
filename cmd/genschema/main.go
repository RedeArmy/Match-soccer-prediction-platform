// Command genschema generates the consolidated schema baseline by running all
// migrations against a temporary PostgreSQL container and dumping the resulting
// DDL via pg_dump. The output is written to migrations/baseline/schema.sql.
//
// Run periodically (every 6-12 months, or after any large batch of migrations)
// to keep the baseline current:
//
//	go run ./cmd/genschema                          # writes to migrations/baseline/schema.sql
//	go run ./cmd/genschema -out /custom/path.sql    # override output path
//
// The generated file is committed to the repository and used by MigrateFresh to
// initialise new environments without replaying every individual migration.
//
// Requirements: Docker must be available in the execution environment.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("genschema: %v", err)
	}
}

func run() error {
	out := flag.String("out", defaultBaselinePath(), "path to write schema.sql")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return generate(ctx, *out)
}

// generate is the testable core of run. It starts a temporary PostgreSQL
// container, runs all migrations, dumps the resulting DDL, and writes the
// filtered output to outPath.
func generate(ctx context.Context, outPath string) error {
	log.Println("genschema: starting PostgreSQL container…")
	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("quiniela_schema"),
		tcpostgres.WithUsername("schema"),
		tcpostgres.WithPassword("schema"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	defer func() {
		if tErr := container.Terminate(ctx); tErr != nil {
			log.Printf("genschema: terminate container: %v", tErr)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("connection string: %w", err)
	}

	log.Println("genschema: running migrations…")
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	log.Println("genschema: dumping schema via pg_dump…")
	schema, err := dumpSchema(ctx, container)
	if err != nil {
		return fmt.Errorf("dump schema: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create baseline dir: %w", err)
	}

	header := schemaHeader()
	content := header + schema
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	log.Printf("genschema: baseline written to %s (%d bytes)", outPath, len(content))
	return nil
}

// dumpSchema executes pg_dump inside the container and returns the DDL output.
// The reader returned by container.Exec carries Docker's multiplexed-stream
// framing; stdcopy.StdCopy is used to extract only the stdout payload.
func dumpSchema(ctx context.Context, container *tcpostgres.PostgresContainer) (string, error) {
	exitCode, muxReader, err := container.Exec(ctx, []string{
		"pg_dump",
		"--schema-only",
		"--no-owner",
		"--no-privileges",
		"--no-comments",
		"-U", "schema",
		"-d", "quiniela_schema",
	})
	if err != nil {
		return "", fmt.Errorf("pg_dump exec: %w", err)
	}

	// Demultiplex Docker's framed stream into stdout / stderr.
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, muxReader); err != nil {
		// Some testcontainers versions return a plain reader — fall back to
		// direct copy when stdcopy encounters a malformed frame header.
		var fallback bytes.Buffer
		_, _ = io.Copy(&fallback, muxReader)
		stdout = fallback
	}
	if exitCode != 0 {
		return "", fmt.Errorf("pg_dump exited %d: %s", exitCode, stderr.String())
	}

	return filterDumpLines(stdout.String()), nil
}

// filterDumpLines removes pg_dump noise lines from raw DDL output.
// It strips SET ..., SELECT pg_catalog ..., and backslash-prefixed psql
// meta-commands (e.g. \connect, \restrict) that are not part of the schema DDL.
func filterDumpLines(raw string) string {
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	keep := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "SET ") ||
			strings.HasPrefix(trimmed, "SELECT pg_catalog") ||
			strings.HasPrefix(trimmed, "\\") {
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, "\n")
}

// defaultBaselinePath returns migrations/baseline/schema.sql relative to the
// current working directory. The tool is expected to be run from the module
// root (go run ./cmd/genschema), which is the standard invocation documented
// in the package comment. Override with -out when running from elsewhere.
func defaultBaselinePath() string {
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join("migrations", "baseline", "schema.sql")
	}
	return filepath.Join(wd, "migrations", "baseline", "schema.sql")
}

func schemaHeader() string {
	return fmt.Sprintf(`-- migrations/baseline/schema.sql
-- AUTO-GENERATED by cmd/genschema — do not edit by hand.
-- Generated: %s
--
-- This file is the consolidated DDL baseline for the quiniela schema.
-- It reflects the state of the database after all migrations up to the
-- most recent one have been applied.
--
-- Usage (new environment fast-path):
--
--   1. Apply this file directly:
--        psql "$DATABASE_URL" -f migrations/baseline/schema.sql
--
--   2. Mark all baseline migrations as applied so golang-migrate does not
--      re-run them on the next startup:
--        go run ./cmd/migrate-mark-applied
--
--   3. Start the application normally; it will run only the migrations
--      added after this baseline was generated.
--
-- Existing environments (already running in production) continue to use
-- the standard Migrate() path and are unaffected by this file.
--

`, time.Now().UTC().Format(time.RFC3339))
}
