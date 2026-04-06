// Package testutil provides shared helpers for integration tests that require
// a live PostgreSQL instance. It uses testcontainers-go to spin up a throwaway
// container per test, keeping tests hermetic and independent of any external
// database infrastructure.
package testutil

import (
	"context"
	"testing"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	// DBImage is the PostgreSQL Docker image used by all integration test containers.
	DBImage = "postgres:17-alpine"
	// DBName is the database name created inside the test container.
	DBName = "quiniela_test"
	// DBUser is the superuser created inside the test container.
	DBUser = "test"
	// DBPassword is the password for DBUser inside the test container.
	DBPassword = "test"
)

// SetupPostgres starts a throwaway PostgreSQL container and returns its DSN.
// The container is terminated automatically when the test finishes via t.Cleanup.
func SetupPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, DBImage,
		tcpostgres.WithDatabase(DBName),
		tcpostgres.WithUsername(DBUser),
		tcpostgres.WithPassword(DBPassword),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	return dsn
}
