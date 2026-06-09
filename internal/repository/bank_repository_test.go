package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

// cleanBankTables wipes gt_banks and bank_account_types before each test so
// migration-seeded rows do not interfere with count or uniqueness assertions.
func cleanBankTables(t *testing.T) {
	t.Helper()
	skipIfNoDB(t)
	_, err := testDB.Exec(context.Background(),
		`TRUNCATE gt_banks, bank_account_types RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("clean bank tables: %v", err)
	}
}

func TestBankRepository_Create_PopulatesIDAndDefaultsToActive(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankRepository(testDB)

	b, err := repo.Create(context.Background(), "Banco Industrial")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if b.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if b.Name != "Banco Industrial" {
		t.Errorf("name: got %q, want %q", b.Name, "Banco Industrial")
	}
	if !b.Active {
		t.Error("expected new bank to be active")
	}
}

func TestBankRepository_ListActive_ReturnsOnlyActiveRows(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankRepository(testDB)

	_, err := testDB.Exec(context.Background(),
		`INSERT INTO gt_banks (name, active) VALUES ('Active Bank', TRUE), ('Inactive Bank', FALSE)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 active bank, got %d", len(result))
	}
	if result[0].Name != "Active Bank" {
		t.Errorf("name: got %q, want %q", result[0].Name, "Active Bank")
	}
}

func TestBankRepository_ListAll_ReturnsAllRows(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankRepository(testDB)

	_, err := testDB.Exec(context.Background(),
		`INSERT INTO gt_banks (name, active) VALUES ('Bank A', TRUE), ('Bank B', FALSE)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 banks, got %d", len(result))
	}
}

func TestBankRepository_SetActive_DisablesBank(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankRepository(testDB)

	created, err := repo.Create(context.Background(), "Toggle Bank")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := repo.SetActive(context.Background(), created.ID, false)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Active {
		t.Error("expected bank to be inactive after SetActive(false)")
	}
	if updated.ID != created.ID {
		t.Errorf("id: got %d, want %d", updated.ID, created.ID)
	}
}

func TestBankRepository_SetActive_ReenablesBank(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankRepository(testDB)

	_, err := testDB.Exec(context.Background(),
		`INSERT INTO gt_banks (name, active) VALUES ('Dormant Bank', FALSE)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	var id int
	_ = testDB.QueryRow(context.Background(),
		`SELECT id FROM gt_banks WHERE name = 'Dormant Bank'`).Scan(&id)

	updated, err := repo.SetActive(context.Background(), id, true)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !updated.Active {
		t.Error("expected bank to be active after SetActive(true)")
	}
}

func TestBankRepository_SetActive_NotFound(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankRepository(testDB)

	_, err := repo.SetActive(context.Background(), 999999, true)
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}
