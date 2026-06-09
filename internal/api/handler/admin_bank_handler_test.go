package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubAdminBankStore struct {
	bank  repository.Bank
	banks []repository.Bank
	err   error
}

func (s *stubAdminBankStore) ListActive(_ context.Context) ([]repository.Bank, error) {
	return s.banks, s.err
}
func (s *stubAdminBankStore) ListAll(_ context.Context) ([]repository.Bank, error) {
	return s.banks, s.err
}
func (s *stubAdminBankStore) Create(_ context.Context, name string) (repository.Bank, error) {
	if s.err != nil {
		return repository.Bank{}, s.err
	}
	return repository.Bank{ID: 1, Name: name, Active: true}, nil
}
func (s *stubAdminBankStore) SetActive(_ context.Context, _ int, active bool) (repository.Bank, error) {
	return repository.Bank{ID: s.bank.ID, Name: s.bank.Name, Active: active}, s.err
}

type stubAdminAccountTypeStore struct {
	typ  repository.BankAccountType
	typs []repository.BankAccountType
	err  error
}

func (s *stubAdminAccountTypeStore) ListActive(_ context.Context) ([]repository.BankAccountType, error) {
	return s.typs, s.err
}
func (s *stubAdminAccountTypeStore) ListAll(_ context.Context) ([]repository.BankAccountType, error) {
	return s.typs, s.err
}
func (s *stubAdminAccountTypeStore) Create(_ context.Context, name string) (repository.BankAccountType, error) {
	if s.err != nil {
		return repository.BankAccountType{}, s.err
	}
	return repository.BankAccountType{ID: 1, Name: name, Active: true}, nil
}
func (s *stubAdminAccountTypeStore) SetActive(_ context.Context, _ int, active bool) (repository.BankAccountType, error) {
	return repository.BankAccountType{ID: s.typ.ID, Name: s.typ.Name, Active: active}, s.err
}

// ── helper ────────────────────────────────────────────────────────────────────

func adminBankRouter(t *testing.T, bankStore *stubAdminBankStore, typeStore *stubAdminAccountTypeStore) http.Handler {
	t.Helper()
	h := handler.NewAdminBankHandler(bankStore, typeStore, zaptest.NewLogger(t))
	r := chi.NewRouter()
	r.Get("/admin/banks", h.ListBanks)
	r.Post("/admin/banks", h.CreateBank)
	r.Patch("/admin/banks/{id}/active", h.SetBankActive)
	r.Get("/admin/bank-account-types", h.ListAccountTypes)
	r.Post("/admin/bank-account-types", h.CreateAccountType)
	r.Patch("/admin/bank-account-types/{id}/active", h.SetAccountTypeActive)
	return r
}

// ── ListBanks ─────────────────────────────────────────────────────────────────

func TestAdminBankHandler_ListBanks_All(t *testing.T) {
	banks := []repository.Bank{
		{ID: 1, Name: "Bancafe", Active: true},
		{ID: 2, Name: "BAM", Active: false},
	}
	router := adminBankRouter(t, &stubAdminBankStore{banks: banks}, &stubAdminAccountTypeStore{})
	req := httptest.NewRequest(http.MethodGet, "/admin/banks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("len: got %d, want 2", len(resp))
	}
}

func TestAdminBankHandler_ListBanks_ActiveFilter(t *testing.T) {
	banks := []repository.Bank{{ID: 1, Name: "Bancafe", Active: true}}
	router := adminBankRouter(t, &stubAdminBankStore{banks: banks}, &stubAdminAccountTypeStore{})
	req := httptest.NewRequest(http.MethodGet, "/admin/banks?active=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestAdminBankHandler_ListBanks_RepoError(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{err: errors.New("db")}, &stubAdminAccountTypeStore{})
	req := httptest.NewRequest(http.MethodGet, "/admin/banks", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 on repo error")
	}
}

// ── CreateBank ────────────────────────────────────────────────────────────────

func TestAdminBankHandler_CreateBank_OK(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{})
	body := bytes.NewBufferString(`{"name":"Nuevo Banco"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/banks", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
}

func TestAdminBankHandler_CreateBank_MissingName(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{})
	body := bytes.NewBufferString(`{"name":""}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/banks", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", w.Code)
	}
}

func TestAdminBankHandler_CreateBank_RepoError(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{err: errors.New("unique violation")}, &stubAdminAccountTypeStore{})
	body := bytes.NewBufferString(`{"name":"Banco X"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/banks", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusCreated {
		t.Fatal("expected non-201 on repo error")
	}
}

// ── SetBankActive ─────────────────────────────────────────────────────────────

func TestAdminBankHandler_SetBankActive_OK(t *testing.T) {
	router := adminBankRouter(t,
		&stubAdminBankStore{bank: repository.Bank{ID: 3, Name: "G&T Continental"}},
		&stubAdminAccountTypeStore{},
	)
	body := bytes.NewBufferString(`{"active":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/admin/banks/3/active", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["active"] != false {
		t.Errorf("active: got %v, want false", resp["active"])
	}
}

func TestAdminBankHandler_SetBankActive_InvalidID(t *testing.T) {
	// Call handler directly (no chi router) so URL param is empty → parse fails.
	h := handler.NewAdminBankHandler(&stubAdminBankStore{}, &stubAdminAccountTypeStore{}, zaptest.NewLogger(t))
	body := bytes.NewBufferString(`{"active":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/admin/banks/abc/active", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SetBankActive(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 for invalid ID")
	}
}

// ── ListAccountTypes ──────────────────────────────────────────────────────────

func TestAdminBankHandler_ListAccountTypes_All(t *testing.T) {
	typs := []repository.BankAccountType{{ID: 1, Name: "Monetaria", Active: true}}
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{typs: typs})
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-account-types", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("len: got %d, want 1", len(resp))
	}
}

func TestAdminBankHandler_ListAccountTypes_ActiveFilter(t *testing.T) {
	typs := []repository.BankAccountType{{ID: 1, Name: "Ahorros", Active: true}}
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{typs: typs})
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-account-types?active=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestAdminBankHandler_ListAccountTypes_RepoError(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{err: errors.New("db")})
	req := httptest.NewRequest(http.MethodGet, "/admin/bank-account-types", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 on repo error")
	}
}

// ── CreateAccountType ─────────────────────────────────────────────────────────

func TestAdminBankHandler_CreateAccountType_OK(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{})
	body := bytes.NewBufferString(`{"name":"Ahorro Dólares"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/bank-account-types", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
}

func TestAdminBankHandler_CreateAccountType_MissingName(t *testing.T) {
	router := adminBankRouter(t, &stubAdminBankStore{}, &stubAdminAccountTypeStore{})
	body := bytes.NewBufferString(`{"name":""}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/bank-account-types", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", w.Code)
	}
}

// ── SetAccountTypeActive ──────────────────────────────────────────────────────

func TestAdminBankHandler_SetAccountTypeActive_OK(t *testing.T) {
	router := adminBankRouter(t,
		&stubAdminBankStore{},
		&stubAdminAccountTypeStore{typ: repository.BankAccountType{ID: 5, Name: "Corriente"}},
	)
	body := bytes.NewBufferString(`{"active":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/admin/bank-account-types/5/active", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestAdminBankHandler_SetAccountTypeActive_InvalidID(t *testing.T) {
	h := handler.NewAdminBankHandler(&stubAdminBankStore{}, &stubAdminAccountTypeStore{}, zaptest.NewLogger(t))
	body := bytes.NewBufferString(`{"active":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/admin/bank-account-types/bad/active", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SetAccountTypeActive(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 for invalid ID")
	}
}
