package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

func TestPostgresTeamRepository_ListTeamNames_ReturnsMigrationSeedData(t *testing.T) {
	skipIfNoDB(t)

	repo := repository.NewPostgresTeamRepository(testDB)
	names, err := repo.ListTeamNames(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// Migration 000167 seeds 48 FIFA 2026 nations.
	if len(names) == 0 {
		t.Fatal("expected seeded team names, got empty slice")
	}
	for _, n := range names {
		if n == "" {
			t.Fatal("empty string in team names result")
		}
	}
}

func TestPostgresTeamRepository_ListTeamNames_IsSortedAZ(t *testing.T) {
	skipIfNoDB(t)

	repo := repository.NewPostgresTeamRepository(testDB)
	names, err := repo.ListTeamNames(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("not sorted at index %d: %q before %q", i, names[i-1], names[i])
		}
	}
}

func TestPostgresTeamRepository_ListTeamNames_NeverNil(t *testing.T) {
	skipIfNoDB(t)

	repo := repository.NewPostgresTeamRepository(testDB)
	names, err := repo.ListTeamNames(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if names == nil {
		t.Fatal("ListTeamNames must return non-nil slice, got nil")
	}
}
