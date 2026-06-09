package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

func TestBankAccountTypeRepository_Create_PopulatesIDAndDefaultsToActive(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankAccountTypeRepository(testDB)

	bt, err := repo.Create(context.Background(), "Ahorros GTQ")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if bt.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if bt.Name != "Ahorros GTQ" {
		t.Errorf("name: got %q, want %q", bt.Name, "Ahorros GTQ")
	}
	if !bt.Active {
		t.Error("expected new account type to be active")
	}
}

func TestBankAccountTypeRepository_ListActive_ReturnsOnlyActiveRows(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankAccountTypeRepository(testDB)

	_, err := testDB.Exec(context.Background(),
		`INSERT INTO bank_account_types (name, active) VALUES ('Monetaria GTQ', TRUE), ('Monetaria USD', FALSE)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 active type, got %d", len(result))
	}
	if result[0].Name != "Monetaria GTQ" {
		t.Errorf("name: got %q, want %q", result[0].Name, "Monetaria GTQ")
	}
}

func TestBankAccountTypeRepository_ListAll_ReturnsAllRows(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankAccountTypeRepository(testDB)

	_, err := testDB.Exec(context.Background(),
		`INSERT INTO bank_account_types (name, active) VALUES ('Type A', TRUE), ('Type B', FALSE)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 types, got %d", len(result))
	}
}

func TestBankAccountTypeRepository_SetActive_DisablesType(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankAccountTypeRepository(testDB)

	created, err := repo.Create(context.Background(), "Toggle Type")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := repo.SetActive(context.Background(), created.ID, false)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Active {
		t.Error("expected type to be inactive after SetActive(false)")
	}
	if updated.ID != created.ID {
		t.Errorf("id: got %d, want %d", updated.ID, created.ID)
	}
}

func TestBankAccountTypeRepository_SetActive_ReenablesType(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankAccountTypeRepository(testDB)

	_, err := testDB.Exec(context.Background(),
		`INSERT INTO bank_account_types (name, active) VALUES ('Dormant Type', FALSE)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	var id int
	_ = testDB.QueryRow(context.Background(),
		`SELECT id FROM bank_account_types WHERE name = 'Dormant Type'`).Scan(&id)

	updated, err := repo.SetActive(context.Background(), id, true)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !updated.Active {
		t.Error("expected type to be active after SetActive(true)")
	}
}

func TestBankAccountTypeRepository_SetActive_NotFound(t *testing.T) {
	cleanBankTables(t)
	repo := repository.NewPostgresBankAccountTypeRepository(testDB)

	_, err := repo.SetActive(context.Background(), 999999, true)
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}
